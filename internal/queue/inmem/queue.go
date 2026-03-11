package inmem

import (
	"log/slog"
	"sync"
	"time"
)

type Queue struct {
	mu            sync.Mutex
	cond          *sync.Cond
	ready         []DelayedMessage
	inflight      map[string]Lease
	inflightCount map[string]int
	dlq           []Message
	shuttingDown  bool

	totalPublished       uint64
	totalAcked           uint64
	totalRedelivered     uint64
	totalNacked          uint64
	totalDLQ             uint64
	totalProcessed       uint64
	totalProcessingNanos uint64

	maxRetries int
	maxDLQSize int
	timeout    time.Duration

	logger *slog.Logger
}

func NewQueue(logger *slog.Logger) *Queue {
	q := &Queue{
		ready:         make([]DelayedMessage, 0),
		inflight:      make(map[string]Lease),
		inflightCount: make(map[string]int),
		dlq:           make([]Message, 0),
		maxRetries:    3,
		maxDLQSize:    100,
		timeout:       5 * time.Second,
		logger:        logger,
	}
	q.cond = sync.NewCond(&q.mu)
	go q.reaper()
	return q
}

func (q *Queue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.ready) + len(q.inflight)
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

func (q *Queue) Snapshot() Metrics {
	q.mu.Lock()
	defer q.mu.Unlock()
	avg := 0.0
	if q.totalProcessed > 0 {
		avg = float64(q.totalProcessingNanos) /
			float64(q.totalProcessed) /
			1e6
	}

	return Metrics{
		Ready:                len(q.ready),
		Inflight:             len(q.inflight),
		DLQ:                  len(q.dlq),
		TotalPublished:       q.totalPublished,
		TotalAcked:           q.totalAcked,
		TotalRedelivered:     q.totalRedelivered,
		TotalNacked:          q.totalNacked,
		TotalDLQ:             q.totalDLQ,
		TotalProcessed:       q.totalProcessed,
		AverageLatencyMillis: avg,
	}
}
