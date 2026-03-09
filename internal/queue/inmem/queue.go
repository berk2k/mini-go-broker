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
	mu            sync.Mutex
	cond          *sync.Cond
	ready         []Message
	inflight      map[string]Lease
	inflightCount map[string]int // consumerID -> count
	dlq           []Message
	shuttingDown  bool
	maxRetries    int
	maxDLQSize    int
	timeout       time.Duration
}

func (q *Queue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.ready) + len(q.inflight)
}

func NewQueue() *Queue {
	q := &Queue{
		ready:         make([]Message, 0),
		inflight:      make(map[string]Lease),
		inflightCount: make(map[string]int),
		dlq:           make([]Message, 0),
		maxRetries:    3,
		maxDLQSize:    100,
		timeout:       5 * time.Second, // visibility timeout
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

func (q *Queue) DequeueLeaseBlocking(consumerID string, prefetch int) (string, Message) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.ready) == 0 || q.inflightCount[consumerID] >= prefetch {
		q.cond.Wait()
	}

	if q.shuttingDown {
		return "", Message{}
	}

	msg := q.ready[0]
	q.inflightCount[consumerID]++

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
	q.inflightCount[consumerID]--
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

		if lease.Message.Attempts >= q.maxRetries {
			q.addToDLQ(lease.Message)
		} else {
			q.ready = append(q.ready, lease.Message)
			q.cond.Signal()
		}
	}

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
				q.inflightCount[lease.ConsumerID]--

				if lease.Message.Attempts >= q.maxRetries {
					q.addToDLQ(lease.Message)
				} else {
					q.ready = append(q.ready, lease.Message)
					q.cond.Signal()
				}
				delete(q.inflight, id)
			}
		}

		q.mu.Unlock()
	}
}

func (q *Queue) addToDLQ(msg Message) {
	if len(q.dlq) >= q.maxDLQSize {
		q.dlq = q.dlq[1:]
	}
	q.dlq = append(q.dlq, msg)
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
				q.cond.Signal()
			}

			q.inflightCount[consumerID]--
			delete(q.inflight, id)
		}
	}
}

func (q *Queue) Shutdown() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.shuttingDown = true
	q.cond.Broadcast()
}

func (q *Queue) InflightSize() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.inflight)
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
		}

		delete(q.inflight, id)
	}
	q.cond.Broadcast()
}
