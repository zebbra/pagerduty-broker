package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/PagerDuty/go-pagerduty"
	"github.com/juju/ratelimit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/zebbra/pagerduty-broker/internal/lib/counter"
	"github.com/zebbra/pagerduty-broker/internal/lib/metrics"
	"github.com/zebbra/pagerduty-broker/internal/lib/queue"
	"go.uber.org/zap"
)

func init() {
	processCmd.Flags().StringP("ampq.url", "a", "amqp://guest:guest@localhost:5672/", "AMPQ connection URL")
	processCmd.Flags().StringP("web.listen-address", "l", ":8080", "Listen address")
	processCmd.Flags().StringP("pagerduty.routingkey", "k", "", "Routing key used to send events")
	processCmd.Flags().Int64P("pagerduty.sendrate", "r", 120, "Max events to send to PagerDuty per minute")
	processCmd.Flags().BoolP("pagerduty.dry-run", "n", false, "Don't send events to PagerDuty, just log")
	_ = processCmd.MarkFlagRequired("pagerduty.routingkey")

	rootCmd.AddCommand(processCmd)
}

var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Start processing messages",
	RunE: func(cmd *cobra.Command, args []string) error {
		ampqURL, err := cmd.Flags().GetString("ampq.url")

		if err != nil {
			return err
		}

		listenAddr, err := cmd.Flags().GetString("web.listen-address")

		if err != nil {
			return err
		}

		routingKey, err := cmd.Flags().GetString("pagerduty.routingkey")

		if err != nil {
			return err
		}

		sendRate, err := cmd.Flags().GetInt64("pagerduty.sendrate")

		if err != nil {
			return err
		}

		dryRun, err := cmd.Flags().GetBool("pagerduty.dry-run")

		if err != nil {
			return err
		}

		logger, _ := zap.NewProduction()
		defer func(logger *zap.Logger) {
			_ = logger.Sync()
		}(logger)
		sugar := logger.Sugar()

		errorCounter := counter.Counter(0)
		eventsReceivedCounter := counter.Counter(0)
		eventsSentCounter := counter.Counter(0)

		go func() {
			sendLimiter := ratelimit.NewBucketWithQuantum(time.Minute, sendRate, sendRate)
			connectLimiter := ratelimit.NewBucketWithQuantum(time.Second, 5, 5)

			sugar.Infof("[worker] Start worker thread to process queue")

			for {
				connectLimiter.Wait(1)
				q, err := queue.NewQueue(ampqURL)

				if err != nil {
					sugar.Errorw("[worker] Error connecting to AMPQ", "error", err)
					errorCounter.Inc()
					continue
				}

				events, err := q.Consume()

				if err != nil {
					sugar.Errorw("[worker] Error connecting to AMPQ", "error", err)
					errorCounter.Inc()
					continue
				}

				sugar.Infof("[worker] Connected to AMQP queue")

				for d := range events {
					if sendLimiter.Available() <= 0 {
						sugar.Infof("[worker] Rate limit reached, throttling...")
					}

					sendLimiter.Wait(1)

					event := &pagerduty.V2Event{}
					err := json.Unmarshal([]byte(d.Body), event)

					if err != nil {
						sugar.Errorw("[worker] Invalid message", "message", string(d.Body), "error", err)
						sugar.Errorw("[worker] Removing from queue")
						_ = d.Nack(false, false)
						errorCounter.Inc()
						continue
					}

					sugar.Infow("[worker] Received message", "message", string(d.Body))
					sugar.Infow("[worker] Parsed event", "event", event.Payload.Summary)

					// override routing key
					event.RoutingKey = routingKey

					// send to pagerduty
					if !dryRun {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()

						response, err := pagerduty.ManageEventWithContext(ctx, *event)

						if err != nil {
							sugar.Errorw("[worker] Error sending event to PagerDuty, send back to queue", "error", err)
							errorCounter.Inc()
							_ = d.Nack(false, true)
							continue
						}

						sugar.Infow("[worker] Sent event to PagerDuty",
							"event", event.Payload.Summary,
							"routing_key", event.RoutingKey,
							"status", response.Status,
							"message", response.Message,
						)
					} else {
						sugar.Infow("[worker] DRY RUN, didn't send event to PagerDuty",
							"event", event.Payload.Summary,
							"routing_key", event.RoutingKey,
						)
					}

					eventsSentCounter.Inc()

					if err := d.Ack(false); err != nil {
						sugar.Errorw("[worker] Error acknowledging message", "error", err)
						errorCounter.Inc()
						continue
					}
				}

				sugar.Error("[worker] Connection to AMQP server lost, reconnecting")
			}
		}()

		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			if errorCounter.Get() > 100 {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, "Unhealthy")
				return
			}

			fmt.Fprint(w, "OK")
		})

		http.HandleFunc("/enqueue", func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			event := &pagerduty.V2Event{}

			err := decoder.Decode(event)

			if err != nil {
				sugar.Errorw("[http] Error parsing event", "error", err)
				http.Error(w, "Error parsing event", http.StatusBadRequest)
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			sugar.Infow("[http] Received event", "event", event.Payload.Summary)
			body, err := json.Marshal(event)

			if err != nil {
				sugar.Errorw("[http] Error encoding event", "error", err)
				http.Error(w, "Error encoding event", http.StatusInternalServerError)
				errorCounter.Inc()
				return
			}

			q, err := queue.NewQueue(ampqURL)

			if err != nil {
				sugar.Errorw("[http] Error connecting to AMPQ", "error", err)
				http.Error(w, "Error connecting to AMPQ", http.StatusBadGateway)
				errorCounter.Inc()
				return
			}

			if err := q.Send(ctx, string(body)); err != nil {
				sugar.Errorw("[http] Error queuing event", "error", err)
				http.Error(w, "Error queuing event", http.StatusBadGateway)
				errorCounter.Inc()
				return
			}

			sugar.Infow("[http] Event enqueued", "event", event.Payload.Summary)
			eventsReceivedCounter.Inc()

			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, "Enqueued")
		})

		// metrics
		reg := prometheus.NewPedanticRegistry()
		_ = reg.Register(collectors.NewBuildInfoCollector())
		_ = reg.Register(collectors.NewGoCollector())
		_ = reg.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
		_ = reg.Register(&metrics.StatisticsCollector{
			ErrorCounter:          &errorCounter,
			EventsReceivedCounter: &eventsReceivedCounter,
			EventsSentCounter:     &eventsSentCounter,
		})

		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

		sugar.Infow("[main] Start listening events", "address", listenAddr)
		return http.ListenAndServe(listenAddr, nil)
	},
}
