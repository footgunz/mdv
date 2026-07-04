package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dgunther/mdv/internal/render"
)

// Hub fans a single reload signal out to all connected SSE clients.
type Hub struct {
	mu      sync.Mutex
	clients map[chan struct{}]bool
}

func NewHub() *Hub { return &Hub{clients: map[chan struct{}]bool{}} }

func (h *Hub) Broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.clients[ch] = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()
	for {
		select {
		case <-ch:
			w.Write([]byte("data: reload\n\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// Server renders Markdown files under baseDir and serves them plus their
// sibling assets. All rendering happens on demand.
type Server struct {
	baseDir string
	hub     *Hub
	mu      sync.Mutex
	current string
	onNav   func(abs string)
}

func NewServer(baseDir string, hub *Hub) *Server {
	return &Server{baseDir: baseDir, hub: hub}
}

func (s *Server) SetOnNav(fn func(abs string)) { s.onNav = fn }

func (s *Server) Current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/_events", s.hub)
	mux.Handle("/_assets/", http.StripPrefix("/_assets/", http.FileServer(http.FS(render.Assets()))))
	mux.HandleFunc("/_user.css", func(w http.ResponseWriter, r *http.Request) {
		if cfg.CSS == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, cfg.CSS)
	})
	mux.HandleFunc("/", s.serve)
	return mux
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	// Resolve the request path safely under baseDir (block traversal).
	rel := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
	abs := filepath.Join(s.baseDir, rel)
	if abs != s.baseDir && !strings.HasPrefix(abs, s.baseDir+string(os.PathSeparator)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if strings.HasSuffix(strings.ToLower(abs), ".md") {
		s.serveMarkdown(w, abs)
		return
	}
	http.ServeFile(w, r, abs)
}

func (s *Server) serveMarkdown(w http.ResponseWriter, abs string) {
	src, err := os.ReadFile(abs)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write(renderer.Page([]byte("<h1>not found</h1>"), "not found", false))
		return
	}
	s.mu.Lock()
	s.current = abs
	s.mu.Unlock()
	if s.onNav != nil {
		s.onNav(abs)
	}

	body, fallback, err := renderer.Body(src)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(renderer.Page(body, filepath.Base(abs), fallback))
}
