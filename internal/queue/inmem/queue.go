package inmem

import (
	"sync"
)

type Message struct {
	ID      string
	Payload []byte
}

type Queue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	messages []Message
}

func NewQueue() *Queue {
	q := &Queue{
		messages: make([]Message, 0),
	}

	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *Queue) Enqueue(msg Message) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.messages = append(q.messages, msg)

	//wake one waiting consumer
	q.cond.Signal()
}

func (q *Queue) DequeueBlocking() Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	// check with a loop to prevent Spurious wake up
	for len(q.messages) == 0 {
		q.cond.Wait()
	}

	msg := q.messages[0]
	q.messages = q.messages[1:]

	return msg
}

func (q *Queue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.messages)
}
