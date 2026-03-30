package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/masterphelps/flowboy/internal/config"
	"github.com/masterphelps/flowboy/internal/engine"
)

//go:embed static/*
var staticFiles embed.FS

// Server serves the REST API and WebSocket for the Flowboy web UI.
type Server struct {
	engine     *engine.Engine
	exporter   *engine.Exporter
	config     *config.Config
	configPath string
	port       int
	mux        *http.ServeMux

	// WebSocket broadcast hub.
	wsClients map[*wsClient]struct{}
	wsMu      sync.Mutex
	startTime time.Time
}

// wsClient is a connected WebSocket client.
type wsClient struct {
	send chan []byte
	done chan struct{}
}

// NewServer creates a Server wired to the engine, exporter, and config.
func NewServer(eng *engine.Engine, exp *engine.Exporter, cfg *config.Config, configPath string, port int) *Server {
	s := &Server{
		engine:     eng,
		exporter:   exp,
		config:     cfg,
		configPath: configPath,
		port:       port,
		mux:        http.NewServeMux(),
		wsClients:  make(map[*wsClient]struct{}),
		startTime:  time.Now(),
	}
	s.routes()
	return s
}

// ServeHTTP implements http.Handler so the server can be used with httptest.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server on the configured port.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()

	return srv.ListenAndServe()
}

// StartBroadcast launches goroutines that fan out engine stats and exporter
// stats to all connected WebSocket clients.
func (s *Server) StartBroadcast(ctx context.Context) {
	// Broadcast engine flow stats.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case st, ok := <-s.engine.Stats():
				if !ok {
					return
				}
				data, _ := json.Marshal(map[string]any{
					"type": "flow_stats",
					"data": st,
				})
				s.broadcast(data)
			}
		}
	}()

	// Broadcast exporter stats every second.
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if s.exporter == nil {
					continue
				}
				data, _ := json.Marshal(map[string]any{
					"type": "exporter_stats",
					"data": s.exporter.GetStats(),
				})
				s.broadcast(data)
			}
		}
	}()
}

// broadcast sends a message to all connected WebSocket clients.
func (s *Server) broadcast(msg []byte) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	for c := range s.wsClients {
		select {
		case c.send <- msg:
		default:
			// Client too slow, drop message.
		}
	}
}

// routes registers all HTTP handlers on the mux.
func (s *Server) routes() {
	s.mux.HandleFunc("/api/machines", s.cors(s.handleMachines))
	s.mux.HandleFunc("/api/machines/", s.cors(s.handleMachineByName))

	s.mux.HandleFunc("/api/flows", s.cors(s.handleFlows))
	s.mux.HandleFunc("/api/flows/", s.cors(s.handleFlowByName))

	s.mux.HandleFunc("/api/collectors", s.cors(s.handleCollectors))
	s.mux.HandleFunc("/api/collectors/", s.cors(s.handleCollectorByName))

	s.mux.HandleFunc("/api/engine/status", s.cors(s.handleEngineStatus))
	s.mux.HandleFunc("/api/engine/start", s.cors(s.handleEngineStart))
	s.mux.HandleFunc("/api/engine/stop", s.cors(s.handleEngineStop))

	s.mux.HandleFunc("/api/segments", s.cors(s.handleSegments))

	s.mux.HandleFunc("/api/configs", s.cors(s.handleConfigs))
	s.mux.HandleFunc("/api/configs/", s.cors(s.handleConfigAction))

	s.mux.HandleFunc("/api/anomaly/scenarios", s.cors(s.handleAnomalyScenarios))
	s.mux.HandleFunc("/api/anomaly/active", s.cors(s.handleAnomalyActive))
	s.mux.HandleFunc("/api/anomaly/start", s.cors(s.handleAnomalyStart))
	s.mux.HandleFunc("/api/anomaly/stop", s.cors(s.handleAnomalyStop))
	s.mux.HandleFunc("/api/anomaly/clear", s.cors(s.handleAnomalyClear))

	s.mux.HandleFunc("/ws", s.handleWebSocket)

	// Serve embedded static files (index.html, css/, js/).
	sub, _ := fs.Sub(staticFiles, "static")
	s.mux.Handle("/", http.FileServer(http.FS(sub)))
}

// cors wraps a handler with CORS headers for local dev.
func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// writeJSON encodes v as JSON and writes it to w.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// readJSON decodes JSON from the request body into v.
func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// saveConfig persists the current config to disk.
func (s *Server) saveConfig() error {
	if s.configPath == "" {
		return nil
	}
	return config.SaveConfig(s.config, s.configPath)
}
