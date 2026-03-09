package inmem

import "time"

func (q *Queue) reaper() {
	ticker := time.NewTicker(1 * time.Second)

	for range ticker.C {
		q.mu.Lock()

		now := time.Now()
		for id, lease := range q.inflight {
			if now.After(lease.Deadline) {
				lease.Message.Attempts++
				q.totalRedelivered++
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
	q.totalDLQ++
}
