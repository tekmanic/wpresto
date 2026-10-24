// Package hook exposes a localhost HTTP endpoint that the WordPress "go live"
// plugin calls to trigger a static rebuild. It requires a shared secret and
// never binds to a public interface by default.
package hook

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// BuildFunc performs a full static build. It receives the build ID to stamp.
type BuildFunc func(ctx context.Context, buildID string) error

// Status describes the current or most recent build.
type Status struct {
	BuildID    string    `json:"build_id"`
	Status     string    `json:"status"` // idle | running | success | error
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// Server is the hook HTTP server.
type Server struct {
	listen  string
	secret  []byte
	buildFn BuildFunc

	mu      sync.Mutex
	running bool
	state   Status
}

// New builds a hook Server.
func New(listen string, secret string, buildFn BuildFunc) *Server {
	return &Server{
		listen:  listen,
		secret:  []byte(secret),
		buildFn: buildFn,
		state:   Status{Status: "idle"},
	}
}

// Handler returns the HTTP handler for the hook endpoints.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /go-live", s.handleGoLive)
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	return mux
}

// ServeHTTP runs the server until ctx is cancelled.
func (s *Server) ServeHTTP(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleGoLive(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "build already running"})
		return
	}
	buildID, err := newBuildID()
	if err != nil {
		s.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	s.running = true
	s.state = Status{BuildID: buildID, Status: "running", StartedAt: time.Now().UTC()}
	s.mu.Unlock()

	go func() {
		err := s.buildFn(r.Context(), buildID)
		now := time.Now().UTC()
		s.mu.Lock()
		s.running = false
		if err != nil {
			s.state = Status{BuildID: buildID, Status: "error", StartedAt: s.state.StartedAt, FinishedAt: now, Error: err.Error()}
		} else {
			s.state = Status{BuildID: buildID, Status: "success", StartedAt: s.state.StartedAt, FinishedAt: now}
		}
		s.mu.Unlock()
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"build_id": buildID, "status": "running"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) authorized(r *http.Request) bool {
	provided := []byte(r.Header.Get("X-WPresto-Secret"))
	if len(provided) == 0 || len(s.secret) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare(provided, s.secret) == 1
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func newBuildID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}
