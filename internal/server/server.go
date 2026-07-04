package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dgunther/mdv/internal/render"
)

// Server renders Markdown files under baseDir and serves them plus their
// sibling assets. All rendering happens on demand.
type Server struct {
	baseDir string
	entry   string // basename served at "/" (the file mdv was opened with)
	r       render.Renderer
	userCSS string // optional stylesheet served at /_user.css
	mu      sync.Mutex
	current string
	onNav   func(abs string)
}

func New(baseDir, entry string, r render.Renderer, userCSS string) *Server {
	return &Server{baseDir: baseDir, entry: entry, r: r, userCSS: userCSS}
}

func (s *Server) SetOnNav(fn func(abs string)) { s.onNav = fn }

func (s *Server) Current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/_assets/", http.StripPrefix("/_assets/", http.FileServer(http.FS(render.Assets()))))
	mux.HandleFunc("/_user.css", func(w http.ResponseWriter, r *http.Request) {
		if s.userCSS == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, s.userCSS)
	})
	mux.HandleFunc("/", s.serve)
	return mux
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	// Resolve the request path safely under baseDir (block traversal).
	rel := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
	if rel == "" {
		rel = s.entry // the Wails window loads "/", not "/<file>.md"
	}
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
		w.Write(s.r.Page([]byte("<h1>not found</h1>"), "not found", false))
		return
	}
	s.mu.Lock()
	s.current = abs
	s.mu.Unlock()
	if s.onNav != nil {
		s.onNav(abs)
	}

	body, fallback, err := s.r.Body(src)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.r.Page(body, filepath.Base(abs), fallback))
}
