package observability

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/berk2k/mini-go-broker/internal/queue/inmem"
)

func RegisterAdminHandlers(queue *inmem.Queue, logger *slog.Logger) {
	http.HandleFunc("/dlq/purge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		msgs := queue.DrainDLQ()
		logger.Info("dlq_purged", slog.Int("count", len(msgs)))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"purged": len(msgs)})
	})

	http.HandleFunc("/dlq/replay", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		count := queue.ReplayDLQ()
		logger.Info("dlq_replayed", slog.Int("count", count))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"replayed": count})
	})
}
