package queue

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Queue struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	queue      amqp.Queue
}

func (q *Queue) Connect(URL string) error {
	conn, err := amqp.Dial(URL)

	if err != nil {
		return err
	}

	q.connection = conn

	ch, err := q.connection.Channel()

	if err != nil {
		return err
	}

	q.channel = ch

	queue, err := q.channel.QueueDeclare(
		"task_queue",
		true,
		false,
		false,
		false,
		nil,
	)

	if err != nil {
		return err
	}

	q.queue = queue
	return nil
}

func (q *Queue) MessageCount() int {
	qs, err := q.channel.QueueInspect("task_queue")

	if err != nil {
		return 0
	}

	return qs.Messages
}

func (q *Queue) Send(ctx context.Context, body string) error {
	return q.channel.PublishWithContext(ctx,
		"",
		q.queue.Name,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         []byte(body),
		})
}

func (q *Queue) Consume() (<-chan amqp.Delivery, error) {
	err := q.channel.Qos(
		1,
		0,
		false,
	)

	if err != nil {
		return nil, err
	}

	return q.channel.Consume(
		q.queue.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
}

func (q *Queue) Close() {
	q.channel.Close()
	q.connection.Close()
}

func NewQueue(URL string) (*Queue, error) {
	q := &Queue{}
	err := q.Connect(URL)

	if err != nil {
		return nil, err
	}

	return q, nil
}
