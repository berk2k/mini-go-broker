package inmem

import "time"

func (q *Queue) RequeueAllForConsumer(consumerID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for id, lease := range q.inflight {
		if lease.ConsumerID == consumerID {

			lease.Message.Attempts++

			q.inflightCount[consumerID]--

			if lease.Message.Attempts >= q.maxRetries {
				q.addToDLQ(lease.Message)
			} else {

				q.ready = append(q.ready, DelayedMessage{
					Message: lease.Message,
					ReadyAt: time.Now(),
				})

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
			q.addToDLQ(lease.Message)
		} else {
			q.ready = append(q.ready, DelayedMessage{
				Message: lease.Message,
				ReadyAt: time.Now(),
			})
		}

		delete(q.inflight, id)
	}

	q.cond.Broadcast()
}
