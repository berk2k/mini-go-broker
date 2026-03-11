package inmem

import (
	"log/slog"
	"time"
)

func (q *Queue) RequeueAllForConsumer(consumerID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for id, lease := range q.inflight {
		if lease.ConsumerID == consumerID {
			lease.Message.Attempts++
			q.inflightCount[consumerID]--

			if lease.Message.Attempts >= q.maxRetries {
				q.logger.Warn("message_dlq",
					slog.String("messageID", lease.Message.ID),
					slog.String("reason", "consumer_disconnect_max_retries"),
					slog.Int("attempts", lease.Message.Attempts),
				)
				q.addToDLQ(lease.Message)
			} else {
				q.ready = append(q.ready, DelayedMessage{
					Message: lease.Message,
					ReadyAt: time.Now(),
				})

				q.logger.Info("message_requeued",
					slog.String("messageID", lease.Message.ID),
					slog.String("reason", "consumer_disconnect"),
					slog.Int("attempt", lease.Message.Attempts),
				)

				q.cond.Signal()
			}

			delete(q.inflight, id)
		}
	}
}

func (q *Queue) ForceRequeueAll() {
	q.mu.Lock()
	defer q.mu.Unlock()

	for id, lease := range q.inflight {
		lease.Message.Attempts++

		if lease.Message.Attempts >= q.maxRetries {
			q.logger.Warn("message_dlq",
				slog.String("messageID", lease.Message.ID),
				slog.String("reason", "force_requeue_max_retries"),
				slog.Int("attempts", lease.Message.Attempts),
			)
			q.addToDLQ(lease.Message)
		} else {
			q.ready = append(q.ready, DelayedMessage{
				Message: lease.Message,
				ReadyAt: time.Now(),
			})

			q.logger.Info("message_requeued",
				slog.String("messageID", lease.Message.ID),
				slog.String("reason", "force_requeue"),
				slog.Int("attempt", lease.Message.Attempts),
			)
		}

		delete(q.inflight, id)
	}

	q.cond.Broadcast()
}
