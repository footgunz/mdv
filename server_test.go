package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerRendersMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# Hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(dir, NewHub())
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
	srv := NewServer(t.TempDir(), NewHub())
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
	srv := NewServer(t.TempDir(), NewHub())
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

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
