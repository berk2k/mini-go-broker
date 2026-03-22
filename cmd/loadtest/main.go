package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	brokerv1 "github.com/berk2k/mini-go-broker/api/proto/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	brokerAddr  = "localhost:50051"
	metricsAddr = "http://localhost:8080/metrics/json"
)

// ─────────────────────────────────────────────
// Config
// ─────────────────────────────────────────────

type Config struct {
	Messages       int
	Prefetch       int
	WaitTimeoutSec int
	ConsumerCounts []int
	QueueName      string
}

// ─────────────────────────────────────────────
// Result
// ─────────────────────────────────────────────

type RunResult struct {
	Consumers   int
	Messages    int
	TotalAcked  int64
	TotalErrors int64
	Duration    time.Duration
	Latencies   []int64 // microseconds
}

func (r *RunResult) Throughput() float64 {
	if r.Duration.Seconds() == 0 {
		return 0
	}
	return float64(r.TotalAcked) / r.Duration.Seconds()
}

func (r *RunResult) Avg() float64 {
	if len(r.Latencies) == 0 {
		return 0
	}
	var sum int64
	for _, l := range r.Latencies {
		sum += l
	}
	return float64(sum) / float64(len(r.Latencies)) / 1000.0 // µs → ms
}

func (r *RunResult) Percentile(p float64) float64 {
	if len(r.Latencies) == 0 {
		return 0
	}
	sorted := make([]int64, len(r.Latencies))
	copy(sorted, r.Latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	return float64(sorted[idx]) / 1000.0 // µs → ms
}

func (r *RunResult) LossPct() float64 {
	if r.Messages == 0 {
		return 0
	}
	loss := float64(r.Messages-int(r.TotalAcked)) / float64(r.Messages) * 100
	if loss < 0 {
		return 0
	}
	return loss
}

// ─────────────────────────────────────────────
// Queue drain check via metrics endpoint
// ─────────────────────────────────────────────

type metricsSnapshot struct {
	Ready    int `json:"ready"`
	Inflight int `json:"inflight"`
}

// waitQueueEmpty polls /metrics/json until ready+inflight == 0 or timeout.
func waitQueueEmpty(timeoutSec int) bool {
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(metricsAddr)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		var snap metricsSnapshot
		if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
			resp.Body.Close()
			time.Sleep(200 * time.Millisecond)
			continue
		}
		resp.Body.Close()
		if snap.Ready == 0 && snap.Inflight == 0 {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// ─────────────────────────────────────────────
// Consumer
// ─────────────────────────────────────────────

func runConsumer(
	client brokerv1.BrokerServiceClient,
	consumerID string,
	queue string,
	prefetch int,
	latencyCh chan<- int64,
	errorCount *atomic.Int64,
	ackedCount *atomic.Int64,
	ctx context.Context,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	stream, err := client.Consume(ctx, &brokerv1.ConsumeRequest{
		Queue:      queue,
		ConsumerId: consumerID,
		Prefetch:   uint32(prefetch),
	})
	if err != nil {
		log.Printf("[%s] connect failed: %v", consumerID, err)
		errorCount.Add(1)
		return
	}

	for {
		delivery, err := stream.Recv()
		if err != nil {
			return // context cancelled = normal shutdown
		}

		start := time.Now()
		_, ackErr := client.Ack(context.Background(), &brokerv1.AckRequest{
			Queue:      queue,
			ConsumerId: consumerID,
			DeliveryId: delivery.DeliveryId,
		})
		if ackErr != nil {
			errorCount.Add(1)
			continue
		}

		latencyMicros := time.Since(start).Microseconds()
		ackedCount.Add(1)

		select {
		case latencyCh <- latencyMicros:
		default:
		}
	}
}

// ─────────────────────────────────────────────
// Single test run
// ─────────────────────────────────────────────

func runScenario(cfg Config, consumerCount int) RunResult {
	conn, err := grpc.NewClient(brokerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	client := brokerv1.NewBrokerServiceClient(conn)

	var (
		ackedCount atomic.Int64
		errorCount atomic.Int64
	)
	latencyCh := make(chan int64, cfg.Messages*2)
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < consumerCount; i++ {
		wg.Add(1)
		go runConsumer(
			client,
			fmt.Sprintf("consumer-%d", i+1),
			cfg.QueueName,
			cfg.Prefetch,
			latencyCh,
			&errorCount,
			&ackedCount,
			ctx,
			&wg,
		)
	}

	// Let streams establish before publishing
	time.Sleep(400 * time.Millisecond)

	start := time.Now()
	published := 0
	for i := 0; i < cfg.Messages; i++ {
		_, pubErr := client.Publish(context.Background(), &brokerv1.PublishRequest{
			Queue:   cfg.QueueName,
			Payload: []byte(fmt.Sprintf("msg-%d", i)),
		})
		if pubErr != nil {
			log.Printf("[publish] error: %v", pubErr)
			continue
		}
		published++
	}

	// Wait for all acks or timeout
	deadline := time.Now().Add(time.Duration(cfg.WaitTimeoutSec) * time.Second)
	for time.Now().Before(deadline) {
		if ackedCount.Load() >= int64(published) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	duration := time.Since(start)

	cancel()
	wg.Wait()
	close(latencyCh)

	var latencies []int64
	for l := range latencyCh {
		latencies = append(latencies, l)
	}

	return RunResult{
		Consumers:   consumerCount,
		Messages:    published,
		TotalAcked:  ackedCount.Load(),
		TotalErrors: errorCount.Load(),
		Duration:    duration,
		Latencies:   latencies,
	}
}

// ─────────────────────────────────────────────
// Output
// ─────────────────────────────────────────────

func printResults(results []RunResult) {
	sep := "────────────────────────────────────────────────────────────────────────────────"
	fmt.Println("\n" + sep)
	fmt.Println("  LOAD TEST RESULTS")
	fmt.Println(sep)
	fmt.Printf("  %-10s  %9s  %9s  %9s  %9s  %7s  %6s  %7s\n",
		"Consumers", "msg/sec", "avg(ms)", "p50(ms)", "p99(ms)", "Acked", "Loss%", "Errors",
	)
	fmt.Println(sep)

	for _, r := range results {
		lossMarker := ""
		if r.LossPct() > 0.5 {
			lossMarker = " ⚠"
		}
		fmt.Printf("  %-10d  %9.1f  %9.3f  %9.3f  %9.3f  %7d  %5.1f%%  %7d%s\n",
			r.Consumers,
			r.Throughput(),
			r.Avg(),
			r.Percentile(50),
			r.Percentile(99),
			r.TotalAcked,
			r.LossPct(),
			r.TotalErrors,
			lossMarker,
		)
	}
	fmt.Println(sep)

	peak := results[0]
	for _, r := range results {
		if r.Throughput() > peak.Throughput() {
			peak = r
		}
	}
	fmt.Printf("\n  ✓ Peak throughput: %.0f msg/sec @ %d consumers\n", peak.Throughput(), peak.Consumers)

	for i := 1; i < len(results); i++ {
		prev, curr := results[i-1], results[i]
		if prev.Throughput() == 0 {
			continue
		}
		dropPct := (1 - curr.Throughput()/prev.Throughput()) * 100
		if dropPct >= 10 {
			fmt.Printf("  ⚠ Throughput drop: %d→%d consumers (%.0f→%.0f msg/sec, %.0f%% drop)\n",
				prev.Consumers, curr.Consumers,
				prev.Throughput(), curr.Throughput(), dropPct,
			)
			fmt.Printf("\n  → Profile this bottleneck:\n")
			fmt.Printf("    go tool pprof http://localhost:6060/debug/pprof/profile\n")
			fmt.Printf("    then: top10\n")
			break
		}
	}
	fmt.Println()
}

// ─────────────────────────────────────────────
// Main
// ─────────────────────────────────────────────

func main() {
	messages := flag.Int("messages", 500, "Messages per scenario")
	prefetch := flag.Int("prefetch", 5, "Prefetch per consumer")
	queue := flag.String("queue", "test", "Queue name")
	timeout := flag.Int("timeout", 30, "Wait timeout in seconds per scenario")
	single := flag.Int("single", 0, "Run a single scenario with N consumers (0 = full scaling test)")
	flag.Parse()

	cfg := Config{
		Messages:       *messages,
		Prefetch:       *prefetch,
		QueueName:      *queue,
		WaitTimeoutSec: *timeout,
		ConsumerCounts: []int{3, 5, 10, 20, 50},
	}

	counts := cfg.ConsumerCounts
	if *single > 0 {
		counts = []int{*single}
	}

	fmt.Printf("\n  Broker  : %s\n", brokerAddr)
	fmt.Printf("  Queue   : %s\n", cfg.QueueName)
	fmt.Printf("  Msgs    : %d per run\n", cfg.Messages)
	fmt.Printf("  Prefetch: %d\n", cfg.Prefetch)
	fmt.Printf("  Runs    : %v\n\n", counts)

	var results []RunResult
	for idx, n := range counts {
		// Wait for queue to fully drain before next scenario
		if idx > 0 {
			fmt.Printf("  [waiting for queue drain...] ")
			ok := waitQueueEmpty(10)
			if ok {
				fmt.Println("✓")
			} else {
				fmt.Println("timeout — continuing anyway")
			}
			// Extra buffer for consumer disconnects to propagate on broker side
			time.Sleep(500 * time.Millisecond)
		}

		fmt.Printf("  [%2d consumers] running... ", n)
		r := runScenario(cfg, n)
		results = append(results, r)
		fmt.Printf("%d acked  %6.1f msg/sec  avg %.3fms  p99 %.3fms\n",
			r.TotalAcked, r.Throughput(), r.Avg(), r.Percentile(99),
		)
	}

	if len(results) > 0 {
		printResults(results)
	}
}
