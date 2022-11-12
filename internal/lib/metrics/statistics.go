package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/zebbra/pagerduty-broker/internal/lib/counter"
)

type StatisticsCollector struct {
	ErrorCounter          *counter.Counter
	EventsReceivedCounter *counter.Counter
	EventsSentCounter     *counter.Counter
}

func (c *StatisticsCollector) Run(ctx context.Context) error {
	_ = ctx
	return nil
}

func (c *StatisticsCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *StatisticsCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"pagerduty_broker_errors",
			"Number of errors",
			[]string{},
			nil,
		),
		prometheus.GaugeValue,
		float64(c.ErrorCounter.Get()),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"pagerduty_broker_messages_received",
			"Number of received messages",
			[]string{},
			nil,
		),
		prometheus.GaugeValue,
		float64(c.EventsReceivedCounter.Get()),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"pagerduty_broker_events_sent",
			"Number of events sent to PagerDuty",
			[]string{},
			nil,
		),
		prometheus.GaugeValue,
		float64(c.EventsSentCounter.Get()),
	)
}
