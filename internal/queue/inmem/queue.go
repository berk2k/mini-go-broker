package inmem

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Message struct {
	ID       string
	Payload  []byte
	Attempts int
}

type Lease struct {
	Message    Message
	ConsumerID string
	Deadline   time.Time
}

type Queue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	ready    []Message
	inflight map[string]Lease
	timeout  time.Duration
}

func (q *Queue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.ready) + len(q.inflight)
}

func NewQueue() *Queue {
	q := &Queue{
		ready:    make([]Message, 0),
		inflight: make(map[string]Lease),
		timeout:  5 * time.Second, // visibility timeout
	}
	q.cond = sync.NewCond(&q.mu)

	// start reaper
	go q.reaper()

	return q
}

func (q *Queue) Enqueue(msg Message) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.ready = append(q.ready, msg)
	q.cond.Signal()
}

func (q *Queue) DequeueLeaseBlocking(consumerID string) (string, Message) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.ready) == 0 {
		q.cond.Wait()
	}

	msg := q.ready[0]
	q.ready = q.ready[1:]

	deliveryID := uuid.NewString()

	q.inflight[deliveryID] = Lease{
		Message:    msg,
		ConsumerID: consumerID,
		Deadline:   time.Now().Add(q.timeout),
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

	delete(q.inflight, deliveryID)
	return nil
}

func (q *Queue) reaper() {
	ticker := time.NewTicker(1 * time.Second)

	for range ticker.C {
		q.mu.Lock()

		now := time.Now()
		for id, lease := range q.inflight {
			if now.After(lease.Deadline) {
				// redelivery
				lease.Message.Attempts++
				q.ready = append(q.ready, lease.Message)
				delete(q.inflight, id)
				q.cond.Signal()
			}
		}

		q.mu.Unlock()
	}
}
