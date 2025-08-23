package kafka // producer: sends 3 lines per tick
import (
	"context"
	"fmt"
	kafka2 "github.com/segmentio/kafka-go"
	"time"
)

// MAINLY FOR TESTING

func Producer(ctx context.Context, conn *kafka2.Conn, k *Kafka, errCh chan<- error) {
	ticker := time.NewTicker(3 * time.Second) // Slowed down to 3 seconds for better readability
	defer ticker.Stop()

	counter := 0
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			messages := []string{
				fmt.Sprintf("Message %d at %s", counter, t.Format("15:04:05")),
				fmt.Sprintf("Data payload %d", counter),
				fmt.Sprintf("Event %d processed", counter),
			}
			if err := k.WriteMessages(conn, messages); err != nil {
				errCh <- fmt.Errorf("producer write failed: %w", err)
				// continue; if the broker is briefly unavailable, keep trying
			}
			counter++
		}
	}
}
