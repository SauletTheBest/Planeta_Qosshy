package outbox

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaPublisher sends outbox messages to Kafka.
type KafkaPublisher struct {
	w *kafka.Writer
}

func NewKafkaPublisher(brokers []string) *KafkaPublisher {
	return &KafkaPublisher{w: &kafka.Writer{
		Addr: kafka.TCP(brokers...),
		// The same key always goes to the same partition, where Kafka keeps order:
		// all events of one order are read in the order they happened.
		Balancer: &kafka.Hash{},
		// The broker confirms only when the message is safely written (on every replica
		// that has to have it). Slower than "fire and forget", but nothing is lost.
		RequiredAcks: kafka.RequireAll,
		// Topics are created on purpose (the kafka-init container), not by a typo here.
		AllowAutoTopicCreation: false,
		// Don't wait to fill a batch: the relay already sends events in batches.
		BatchTimeout: 10 * time.Millisecond,
		WriteTimeout: 10 * time.Second,
		// A few quick retries ride out a short hiccup (e.g. a new partition leader).
		// Longer outages are the relay's job: it keeps the events and backs off itself,
		// so the default 10 attempts would only delay noticing that Kafka is down.
		MaxAttempts: 3,
	}}
}

func (p *KafkaPublisher) Publish(ctx context.Context, msgs []Message) error {
	km := make([]kafka.Message, len(msgs))
	for i, m := range msgs {
		km[i] = kafka.Message{Topic: m.Topic, Key: []byte(m.Key), Value: m.Value}
	}
	return p.w.WriteMessages(ctx, km...)
}

func (p *KafkaPublisher) Close() error { return p.w.Close() }
