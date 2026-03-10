package backoff

import "time"

func Exponential(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	base := 500 * time.Millisecond
	delay := base * time.Duration(1<<uint(attempt-1))

	max := 10 * time.Second
	if delay > max {
		return max
	}

	return delay
}
