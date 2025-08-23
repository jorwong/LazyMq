package kafka

import (
	"context"
	"fmt"
	"github.com/segmentio/kafka-go"
	"time"
)

type Kafka struct {
	Topic     string
	partition int
	address   string
	network   string
	groupId   string
}

func NewKafka() *Kafka {
	return &Kafka{
		Topic:     "test",
		partition: 0,
		address:   "localhost:9092",
		network:   "tcp",
	}

}

func NewKafkaCustom(topic string, partition int, address string) *Kafka {
	return &Kafka{
		Topic:     topic,
		partition: partition,
		network:   "tcp",
		address:   address,
	}
}

func (k *Kafka) Connect() (*kafka.Conn, error) {
	conn, err := kafka.DialLeader(
		context.Background(), k.network, k.address, k.Topic, k.partition)
	if err != nil {
		fmt.Printf("failed to dial leader: %v\n", err)
		return nil, err
	}
	return conn, nil
}

func (k *Kafka) WriteMessages(conn *kafka.Conn, msgs []string) error {
	var err error
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	for _, msg := range msgs {
		_, err = conn.WriteMessages(kafka.Message{Value: []byte(msg)})
	}
	if err != nil {
		fmt.Printf("failed to write messages: %v\n", err)
		return err
	}

	return nil
}

func (k *Kafka) TailReader(brokers []string, topic string, partition int) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		Partition:   partition,
		StartOffset: kafka.LastOffset, // <-- start at the end
		// (optional) set MinBytes/MaxBytes, etc.
	})
}

func (k *Kafka) DeleteTopic(conn *kafka.Conn, topic string) error {
	err := conn.DeleteTopics(topic)

	if err != nil {
		return err
	}

	return nil
}
