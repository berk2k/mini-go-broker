package inmem

type Metrics struct {
	Ready                int
	Inflight             int
	DLQ                  int
	TotalPublished       uint64
	TotalAcked           uint64
	TotalRedelivered     uint64
	TotalNacked          uint64
	TotalDLQ             uint64
	TotalProcessed       uint64
	AverageLatencyMillis float64
}
