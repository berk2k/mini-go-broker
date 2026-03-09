package inmem

import "time"

type Message struct {
	ID       string
	Payload  []byte
	Attempts int
}

type Lease struct {
	Message    Message
	ConsumerID string
	Deadline   time.Time
	StartedAt  time.Time
}
