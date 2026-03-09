package observability

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/berk2k/mini-go-broker/internal/queue/inmem"
)

func StartMetricsServer(queue *inmem.Queue, port string) {
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		m := queue.Snapshot()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	log.Printf("Metrics server running on %s\n", port)

	go func() {
		if err := http.ListenAndServe(port, nil); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()
}
