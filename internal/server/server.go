package server

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dgunther/mdv/internal/render"
)

// Server renders Markdown files under baseDir and serves them plus their
// sibling assets. All rendering happens on demand.
type Server struct {
	r       render.Renderer
	userCSS string // optional stylesheet served at /_user.css
	mu      sync.Mutex
	baseDir string
	entry   string // basename served at "/" (the file mdv was opened with)
	current string
	onNav   func(abs string)
}

// New serves entry (a basename under baseDir) at "/". Both may be empty: a
// bundle launched from the Dock has no document until one arrives via
// SetEntry, and renders a placeholder until then.
func New(baseDir, entry string, r render.Renderer, userCSS string) *Server {
	return &Server{baseDir: baseDir, entry: entry, r: r, userCSS: userCSS}
}

// SetEntry re-points "/" at abs. macOS delivers "Open With" documents to an
// already-running app as an Apple Event, so the served file can change after
// startup — and with it baseDir, which is also the traversal boundary.
func (s *Server) SetEntry(abs string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseDir, s.entry = filepath.Dir(abs), filepath.Base(abs)
}

func (s *Server) SetOnNav(fn func(abs string)) { s.onNav = fn }

func (s *Server) Current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// HasEntry reports whether a document has been chosen yet.
func (s *Server) HasEntry() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entry != ""
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
	s.mu.Lock()
	baseDir, entry := s.baseDir, s.entry
	s.mu.Unlock()
	if entry == "" {
		// No document yet: launched from the Dock, waiting on an open event.
		// Bail before resolving anything — an empty baseDir is no boundary.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(s.r.Page([]byte("<h1>mdv</h1><p>Open a Markdown file.</p>"), "mdv", false))
		return
	}

	// Resolve the request path safely under baseDir (block traversal).
	// Clean as a URL path (always "/"-separated), then convert: on Windows
	// filepath.Clean("/") is `\`, which would never match the "/" prefix.
	rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if rel == "" {
		rel = entry // the Wails window loads "/", not "/<file>.md"
	}
	rel = filepath.FromSlash(rel)
	// IsLocal also rejects Windows drive/UNC/reserved names (C:\x, NUL).
	if !filepath.IsLocal(rel) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	abs := filepath.Join(baseDir, rel)

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
