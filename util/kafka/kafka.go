package kafka

import (
	"context"
	"errors"
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

func (k *Kafka) NewTailReader(brokers []string, topic string, partition int) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		Partition:   partition,
		StartOffset: kafka.LastOffset, // <-- start at the end
		// (optional) set MinBytes/MaxBytes, etc.
	})
}

func (k *Kafka) ReadMessages(ctx context.Context, messageChannel chan string, minSize, maxSize int) {
	fmt.Println("Reading messages")

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{k.address},
		Topic:       k.Topic,
		GroupID:     "lazy-consumer",  // OR: Partition: k.partition (pick one)
		StartOffset: kafka.LastOffset, // tail new messages only
		MinBytes:    1,                // 1 byte
		MaxBytes:    10e6,             // 10 MB
		// MaxWait:   time.Second,      // optional: how long to wait to fill MinBytes
	})
	defer r.Close()

	for {
		msg, err := r.ReadMessage(ctx)
		if err != nil {
			// If context was canceled, exit cleanly
			if errors.Is(err, context.Canceled) {
				return
			}
			fmt.Printf("failed to read message: %v\n", err)
			// small backoff to avoid tight loop on repeated errors
			time.Sleep(200 * time.Millisecond)
			continue
		}
		messageChannel <- string(msg.Value)
	}
}

func (k *Kafka) DeleteTopic(conn *kafka.Conn, topic string) error {
	err := conn.DeleteTopics(topic)

	if err != nil {
		return err
	}

	return nil
}
