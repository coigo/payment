package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	TopicPaymentEvents = "payments.events"
	TopicReviewEvents = "reviews.events"
)

func NewWriter (brokers []string, topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr: kafka.TCP(brokers...),
		Topic: topic,
		Balancer: &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks: kafka.RequireNone,
	}
}

func NewReader (brokers []string, topic, groupId string) *kafka.Reader {
	return kafka.NewReader( kafka.ReaderConfig{
		Brokers: brokers,
		Topic: topic,
		GroupID: groupId,
		MinBytes:  1,
		MaxBytes:  10 << 20,
		CommitInterval: time.Second,
	})
}

func EnsureTopics (ctx context.Context, broker string) (err error) {
	client, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		return err
	}
	defer client.Close()

	controller, err := client.Controller()
	if err != nil {
		return fmt.Errorf("fafka: failed to get controller: %w", err)
	}

	ctrl, err := kafka.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("fafka: failed to connect controller: %w", err)
	}

	defer ctrl.Close()

	topics := []kafka.TopicConfig{
		{
			Topic: TopicPaymentEvents,
			NumPartitions: 3,
			ReplicationFactor: 1,
		},
		{
			Topic: TopicReviewEvents,
			NumPartitions: 3,
			ReplicationFactor: 1,
		},
	}

	if err := ctrl.CreateTopics(topics...); err != nil {
		return fmt.Errorf("kafka: failed to create topics %w", err)
	}
	
	return nil
}