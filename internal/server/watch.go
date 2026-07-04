package server

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Reloader watches one directory and calls onChange (debounced) whenever the
// file returned by current() is written. It watches the directory rather than
// the file so editor atomic-saves (rename-over) are still caught.
type Reloader struct {
	w        *fsnotify.Watcher
	current  func() string
	onChange func()

	mu  sync.Mutex
	dir string
}

func NewReloader(current func() string, onChange func()) (*Reloader, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	r := &Reloader{w: w, current: current, onChange: onChange}
	go r.loop()
	return r, nil
}

func (r *Reloader) Watch(dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if dir == r.dir {
		return nil
	}
	if r.dir != "" {
		r.w.Remove(r.dir)
	}
	if err := r.w.Add(dir); err != nil {
		return err
	}
	r.dir = dir
	return nil
}

func (r *Reloader) Close() error { return r.w.Close() }

func (r *Reloader) loop() {
	var timer *time.Timer
	debounced := func() {
		if r.onChange != nil {
			r.onChange()
		}
	}
	for {
		select {
		case ev, ok := <-r.w.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			if filepath.Clean(ev.Name) != filepath.Clean(r.current()) {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(100*time.Millisecond, debounced)
		case _, ok := <-r.w.Errors:
			if !ok {
				return
			}
		}
	}
}
