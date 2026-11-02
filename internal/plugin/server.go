package plugin

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Server is the simple HTTP server that runs the registered plugin handlers
// and proxies any unmatched request to a fallback backend (the WordPress
// origin). It is the target of nginx's dynamic fallback: known dynamic parts
// are served by plugins, everything else goes to WordPress.
type Server struct {
	listen   string
	fallback *url.URL
	client   *http.Client
}

// NewServer builds a plugin Server. fallback may be nil, in which case
// unmatched requests get a 404.
func NewServer(listen string, fallback *url.URL) *Server {
	return &Server{
		listen:   listen,
		fallback: fallback,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

// Listen returns the address the server binds to.
func (s *Server) Listen() string { return s.listen }

// Handler returns the root HTTP handler: plugin routes first, fallback proxy
// for anything unmatched.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := Find(r.URL.Path, r.Method); ok {
			s.servePlugin(w, r, p)
			return
		}
		s.proxy(w, r)
	})
}

// servePlugin runs a plugin's Lambda-style handler and copies the response.
func (s *Server) servePlugin(w http.ResponseWriter, r *http.Request, p Plugin) {
	resp, err := p.Handle(r.Context(), r)
	if err != nil {
		http.Error(w, "plugin error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	if resp.StatusCode != 0 {
		w.WriteHeader(resp.StatusCode)
	}
	_, _ = io.Copy(w, resp.Body)
}

// proxy forwards an unmatched request to the fallback backend.
func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	if s.fallback == nil {
		http.Error(w, "no plugin matched and no fallback configured", http.StatusNotFound)
		return
	}
	out := s.fallback.ResolveReference(r.URL)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, out.String(), r.Body)
	if err != nil {
		http.Error(w, "proxy: "+err.Error(), http.StatusBadGateway)
		return
	}
	req.Header = r.Header.Clone()
	req.Host = s.fallback.Host

	resp, err := s.client.Do(req)
	if err != nil {
		http.Error(w, "proxy: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
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
