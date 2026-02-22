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
	messages []Message
}

func NewQueue() *Queue {
	return &Queue{
		messages: make([]Message, 0),
	}
}

func (q *Queue) Enqueue(msg Message) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.messages = append(q.messages, msg)
}

func (q *Queue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.messages)
}
