package inmem

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

func (q *Queue) Enqueue(msg Message) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.ready = append(q.ready, msg)
	q.totalPublished++
	q.cond.Signal()
}

func (q *Queue) DequeueLeaseBlocking(consumerID string, prefetch int) (string, Message) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for (len(q.ready) == 0 || q.inflightCount[consumerID] >= prefetch) && !q.shuttingDown {
		q.cond.Wait()
	}

	if q.shuttingDown {
		return "", Message{}
	}

	msg := q.ready[0]
	q.ready = q.ready[1:]
	q.inflightCount[consumerID]++

	deliveryID := uuid.NewString()

	q.inflight[deliveryID] = Lease{
		Message:    msg,
		ConsumerID: consumerID,
		Deadline:   time.Now().Add(q.timeout),
		StartedAt:  time.Now(),
	}

	return deliveryID, msg
}

func (q *Queue) Ack(deliveryID string, consumerID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	lease, ok := q.inflight[deliveryID]
	if !ok {
		return errors.New("invalid deliveryID")
	}
	if lease.ConsumerID != consumerID {
		return errors.New("consumer mismatch")
	}

	latency := time.Since(lease.StartedAt)

	q.totalProcessed++
	q.totalProcessingNanos += uint64(latency.Nanoseconds())
	delete(q.inflight, deliveryID)
	q.inflightCount[consumerID]--
	q.totalAcked++
	q.cond.Signal()
	return nil
}

func (q *Queue) Nack(deliveryID string, consumerID string, requeue bool) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	lease, ok := q.inflight[deliveryID]
	if !ok {
		return errors.New("invalid deliveryID")
	}
	if lease.ConsumerID != consumerID {
		return errors.New("consumer mismatch")
	}

	delete(q.inflight, deliveryID)
	q.inflightCount[consumerID]--

	if requeue {
		lease.Message.Attempts++
		q.totalNacked++

		if lease.Message.Attempts >= q.maxRetries {
			q.addToDLQ(lease.Message)
		} else {
			q.ready = append(q.ready, lease.Message)
			q.totalRedelivered++
			q.cond.Signal()
		}
	}

	return nil
}

func (q *Queue) RequeueAllForConsumer(consumerID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for id, lease := range q.inflight {
		if lease.ConsumerID == consumerID {

			lease.Message.Attempts++

			if lease.Message.Attempts >= q.maxRetries {
				q.addToDLQ(lease.Message)
			} else {
				q.ready = append(q.ready, lease.Message)
				q.totalRedelivered++
				q.cond.Signal()
			}

			q.inflightCount[consumerID]--
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
			q.addToDLQ(lease.Message)
		} else {
			q.ready = append(q.ready, lease.Message)
			q.totalRedelivered++
		}

		delete(q.inflight, id)
	}
	q.cond.Broadcast()
}
