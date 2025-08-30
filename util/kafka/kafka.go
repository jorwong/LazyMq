// lazyMq.com/util/kafka/kafka.go
package kafka

import (
	"context"
	"sort"
	"time"

	"github.com/segmentio/kafka-go"
)

type Kafka struct {
	Partition int
	Address   string
	network   string
}

func NewKafkaCustom(partition int, address string) *Kafka {
	return &Kafka{
		Partition: partition,
		Address:   address,
		network:   "tcp",
	}
}

func (k *Kafka) Connect() (*kafka.Conn, error) {
	return kafka.Dial(k.network, k.Address)
}

// ---- Admin helpers ---------------------------------------------------------

func (k *Kafka) ListTopics() ([]string, error) {
	conn, err := k.Connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	parts, err := conn.ReadPartitions()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, p := range parts {
		seen[p.Topic] = struct{}{}
	}
	topics := make([]string, 0, len(seen))
	for t := range seen {
		topics = append(topics, t)
	}
	sort.Strings(topics)
	return topics, nil
}

func (k *Kafka) DeleteTopic(topic string) error {
	conn, err := k.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.DeleteTopics(topic)
}

func (k *Kafka) CreateTopic(topic string) error {
	conn, err := k.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()
	topicConfig := kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     -1,
		ReplicationFactor: -1,
	}
	return conn.CreateTopics(topicConfig)
}

// ---- IO helpers ------------------------------------------------------------

func (k *Kafka) TailReader(brokers []string, topic string, partition int) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		Partition:   partition,
		StartOffset: kafka.LastOffset, // tail
		MinBytes:    1,
		MaxBytes:    10e6,
	})
}

func (k *Kafka) HeadReader(brokers []string, topic string, partition int) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		Partition:   partition,
		StartOffset: kafka.FirstOffset, // head
		MinBytes:    1,
		MaxBytes:    10e6,
	})
}

func (k *Kafka) NewWriter(topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:     kafka.TCP(k.Address),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
		// Async: false (default) for visibility of errors; set to true if you prefer
		BatchTimeout: 50 * time.Millisecond,
	}
}

func (k *Kafka) Produce(ctx context.Context, w *kafka.Writer, msgs ...string) error {
	kmsgs := make([]kafka.Message, len(msgs))
	for i, m := range msgs {
		kmsgs[i] = kafka.Message{Value: []byte(m)}
	}
	return w.WriteMessages(ctx, kmsgs...)
}
