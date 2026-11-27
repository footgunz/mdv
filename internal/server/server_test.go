package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dgunther/mdv/internal/config"
	"github.com/dgunther/mdv/internal/render"
)

// newTestServer builds a Server with default config; userCSS optionally
// points /_user.css at a stylesheet.
func newTestServer(dir, userCSS string) *Server {
	return New(dir, "index.md", render.Renderer{Cfg: config.Default()}, userCSS)
}

func TestServerRendersMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# Hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(dir, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/a.md")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := readAll(t, resp)
	if resp.StatusCode != 200 || !strings.Contains(body, "<h1") {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if srv.Current() != filepath.Join(dir, "a.md") {
		t.Fatalf("current = %q", srv.Current())
	}
}

func TestServerMissingMarkdown(t *testing.T) {
	srv := newTestServer(t.TempDir(), "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nope.md")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 || !strings.Contains(readAll(t, resp), "not found") {
		t.Fatalf("expected 404 not found, got %d", resp.StatusCode)
	}
}

func TestServerServesEmbeddedAsset(t *testing.T) {
	srv := newTestServer(t.TempDir(), "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/_assets/base.css")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestServerUserCSS(t *testing.T) {
	dir := t.TempDir()
	css := filepath.Join(dir, "u.css")
	if err := os.WriteFile(css, []byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(dir, css)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/_user.css")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(readAll(t, resp), "color:red") {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

// SetEntry is how a macOS "Open With" re-points a running instance, possibly
// at a file in another directory — which also moves the traversal boundary.
func TestServerSetEntrySwitchesDirectory(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for path, src := range map[string]string{
		filepath.Join(a, "a.md"): "# Alpha",
		filepath.Join(b, "b.md"): "# Beta",
	} {
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv := New(a, "a.md", render.Renderer{Cfg: config.Default()}, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	if code, body := get(t, ts.URL+"/"); code != 200 || !strings.Contains(body, "Alpha") {
		t.Fatalf("before SetEntry: status %d body %s", code, body)
	}
	srv.SetEntry(filepath.Join(b, "b.md"))
	if code, body := get(t, ts.URL+"/"); code != 200 || !strings.Contains(body, "Beta") {
		t.Fatalf("after SetEntry: status %d body %s", code, body)
	}
	// the old directory is out of bounds now, not merely unlinked
	if code, _ := get(t, ts.URL+"/a.md"); code != 404 {
		t.Fatalf("stale dir still reachable: status %d", code)
	}
}

// A bundle launched with no document has no baseDir, so there is no boundary
// to check against — it must refuse to resolve anything at all.
func TestServerNoEntryServesPlaceholder(t *testing.T) {
	srv := New("", "", render.Renderer{Cfg: config.Default()}, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, path := range []string{"/", "/etc/hosts"} {
		code, body := get(t, ts.URL+path)
		if code != 200 || !strings.Contains(body, "Open a Markdown file") {
			t.Fatalf("GET %s: status %d body %s", path, code, body)
		}
	}
	if srv.HasEntry() {
		t.Fatal("HasEntry true with no document")
	}
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, readAll(t, resp)
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The traversal boundary must hold for every OS's path syntax, not just "/":
// on Windows a drive letter or `\` in the URL path would otherwise escape.
func TestServerRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# Alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := New(dir, "a.md", render.Renderer{Cfg: config.Default()}, "").Handler()
	for _, p := range []string{"/../a.md", `/..\..\a.md`, "/C:/Windows/win.ini", `/\\host\share\x.md`} {
		req := httptest.NewRequest("GET", "/", nil)
		req.URL.Path = p // bypass the mux's own cleaning to hit serve's check
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == 200 {
			t.Errorf("GET %q: status 200, want refusal", p)
		}
	}
}
