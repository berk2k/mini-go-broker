package observability

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/berk2k/mini-go-broker/internal/queue/inmem"
)

func StartMetricsServer(queue *inmem.Queue, port string) {

	http.HandleFunc("/metrics/json", func(w http.ResponseWriter, r *http.Request) {
		m := queue.Snapshot()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		m := queue.Snapshot()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		fmt.Fprintf(w, "# HELP mini_broker_ready Current ready queue size\n")
		fmt.Fprintf(w, "# TYPE mini_broker_ready gauge\n")
		fmt.Fprintf(w, "mini_broker_ready %d\n\n", m.Ready)

		fmt.Fprintf(w, "# HELP mini_broker_inflight Current inflight message count\n")
		fmt.Fprintf(w, "# TYPE mini_broker_inflight gauge\n")
		fmt.Fprintf(w, "mini_broker_inflight %d\n\n", m.Inflight)

		fmt.Fprintf(w, "# HELP mini_broker_dlq Current DLQ size\n")
		fmt.Fprintf(w, "# TYPE mini_broker_dlq gauge\n")
		fmt.Fprintf(w, "mini_broker_dlq %d\n\n", m.DLQ)

		fmt.Fprintf(w, "# HELP mini_broker_total_published Total published messages\n")
		fmt.Fprintf(w, "# TYPE mini_broker_total_published counter\n")
		fmt.Fprintf(w, "mini_broker_total_published %d\n\n", m.TotalPublished)

		fmt.Fprintf(w, "# HELP mini_broker_total_acked Total acked messages\n")
		fmt.Fprintf(w, "# TYPE mini_broker_total_acked counter\n")
		fmt.Fprintf(w, "mini_broker_total_acked %d\n\n", m.TotalAcked)

		fmt.Fprintf(w, "# HELP mini_broker_total_redelivered Total redelivered messages\n")
		fmt.Fprintf(w, "# TYPE mini_broker_total_redelivered counter\n")
		fmt.Fprintf(w, "mini_broker_total_redelivered %d\n\n", m.TotalRedelivered)

		fmt.Fprintf(w, "# HELP mini_broker_total_nacked Total nacked messages\n")
		fmt.Fprintf(w, "# TYPE mini_broker_total_nacked counter\n")
		fmt.Fprintf(w, "mini_broker_total_nacked %d\n\n", m.TotalNacked)

		fmt.Fprintf(w, "# HELP mini_broker_total_dlq Total messages moved to DLQ\n")
		fmt.Fprintf(w, "# TYPE mini_broker_total_dlq counter\n")
		fmt.Fprintf(w, "mini_broker_total_dlq %d\n", m.TotalDLQ)

		fmt.Fprintf(w, "# HELP mini_broker_total_processed Total successfully processed messages\n")
		fmt.Fprintf(w, "# TYPE mini_broker_total_processed counter\n")
		fmt.Fprintf(w, "mini_broker_total_processed %d\n\n", m.TotalProcessed)

		fmt.Fprintf(w, "# HELP mini_broker_avg_processing_latency_ms Average processing latency in milliseconds\n")
		fmt.Fprintf(w, "# TYPE mini_broker_avg_processing_latency_ms gauge\n")
		fmt.Fprintf(w, "mini_broker_avg_processing_latency_ms %.2f\n", m.AverageLatencyMillis)
	})

	log.Printf("Metrics server running on %s\n", port)

	go func() {
		if err := http.ListenAndServe(port, nil); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()
}
