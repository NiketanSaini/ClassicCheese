// Package rabbitmqclient wraps amqp091-go with the topology this POC uses:
// one durable fanout exchange, and durable queues each service binds to
// itself on startup so there's no separate "create the topic" step.
package rabbitmqclient

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Connect opens a connection and a single channel against it.
func Connect(url string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("dialing rabbitmq at %s: %w", url, err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("opening rabbitmq channel: %w", err)
	}
	return conn, ch, nil
}

// DeclareExchange idempotently declares the durable fanout exchange every
// service publishes to / consumes from. Whichever service starts first
// creates it; the rest no-op against the existing declaration.
func DeclareExchange(ch *amqp.Channel, exchange string) error {
	return ch.ExchangeDeclare(
		exchange,
		"fanout",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
}

// DeclareAndBindQueue idempotently declares a durable queue and binds it to
// the exchange. Fanout ignores routing keys, so the binding key is empty.
func DeclareAndBindQueue(ch *amqp.Channel, exchange, queueName string) (amqp.Queue, error) {
	q, err := ch.QueueDeclare(
		queueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("declaring queue %s: %w", queueName, err)
	}
	if err := ch.QueueBind(queueName, "", exchange, false, nil); err != nil {
		return amqp.Queue{}, fmt.Errorf("binding queue %s to exchange %s: %w", queueName, exchange, err)
	}
	return q, nil
}

// Publish sends a persistent JSON message to the fanout exchange. Fanout
// ignores routing keys, so it's always published with an empty one.
func Publish(ch *amqp.Channel, exchange string, body []byte) error {
	return ch.Publish(
		exchange,
		"", // routing key: ignored by fanout
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}

// Consume sets a modest prefetch count and returns a manual-ack delivery
// channel for queueName, so a slow consumer isn't flooded and a crash
// mid-processing causes redelivery instead of silent message loss.
func Consume(ch *amqp.Channel, queueName string, prefetch int) (<-chan amqp.Delivery, error) {
	if err := ch.Qos(prefetch, 0, false); err != nil {
		return nil, fmt.Errorf("setting QoS for %s: %w", queueName, err)
	}
	deliveries, err := ch.Consume(
		queueName,
		"",    // consumer tag
		false, // autoAck
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("consuming from %s: %w", queueName, err)
	}
	return deliveries, nil
}
