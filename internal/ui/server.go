package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/local/picobot/embeds"
	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/config"
	"github.com/local/picobot/internal/cron"
)

// StartServer starts the Web UI server
func StartServer(ctx context.Context, cfgPath string, hub *chat.Hub, scheduler *cron.Scheduler, port int) error {
	mux := http.NewServeMux()

	// 1. Serve static files from embedded FS
	uiFS, err := fs.Sub(embeds.UI, "ui")
	if err != nil {
		return fmt.Errorf("failed to load embedded ui fs: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(uiFS)))

	// 2. Chat API
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Text      string `json:"text"`
			SessionID string `json:"sessionId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Send to hub
		hub.In <- chat.Inbound{
			Channel:   "web",
			SenderID:  payload.SessionID,
			ChatID:    payload.SessionID,
			Content:   payload.Text,
			Timestamp: time.Now(),
		}

		w.WriteHeader(http.StatusOK)
	})

	// 3. Chat stream (SSE)
	mux.HandleFunc("/api/chat/stream", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session")
		outbound := hub.Subscribe("web")

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		// keep alive ticker
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case msg, ok := <-outbound:
				if !ok {
					return
				}
				// only send to the relevant session
				if msg.ChatID == sessionID {
					b, _ := json.Marshal(map[string]interface{}{
						"channel": msg.Channel,
						"chatId":  msg.ChatID,
						"content": msg.Content,
						"replyTo": msg.ReplyTo,
					})
					fmt.Fprintf(w, "data: %s\n\n", string(b))
					flusher.Flush()
				}
			case <-ticker.C:
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	})

	// 4. Config API
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Get fresh config since it might have changed on disk
			cfg, err := config.LoadConfig()
			if err != nil {
				// if error, we can still fall back to default, but let's just return what we have
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfg)
			return
		}
		if r.Method == http.MethodPost {
			var newCfg config.Config
			if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// we need to write to disk.
			if err := config.SaveConfig(newCfg, cfgPath); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	// 5. Cron API
	mux.HandleFunc("/api/cron", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			var jobs []cron.Job
			if scheduler != nil {
				jobs = scheduler.List()
			} else {
				jobs = make([]cron.Job, 0)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jobs)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/cron/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			id := r.URL.Path[len("/api/cron/"):]
			if scheduler != nil && scheduler.Cancel(id) {
				w.WriteHeader(http.StatusOK)
			} else {
				http.Error(w, "Not found", http.StatusNotFound)
			}
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	// Add CORS (optional but good for local dev)
	handler := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			h.ServeHTTP(w, r)
		})
	}(mux)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}

	go func() {
		log.Printf("Starting Web UI on http://localhost:%d\n", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Web UI server error: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
