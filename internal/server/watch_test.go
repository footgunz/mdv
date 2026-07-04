package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReloaderFiresOnChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.md")
	if err := os.WriteFile(file, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	fired := make(chan struct{}, 1)
	r, err := NewReloader(
		func() string { return file },
		func() { fired <- struct{}{} },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Watch(dir); err != nil {
		t.Fatal(err)
	}

	// Give the watcher goroutine a moment, then modify the file.
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(file, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("reload never fired")
	}
}
