package ingestion

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// EventType represents the kind of filesystem event detected.
type EventType int

const (
	EventFileCreated EventType = iota
	EventFileModified
	EventFileDeleted
)

// LogFileEvent represents a change to a .jsonl log file.
type LogFileEvent struct {
	Type EventType
	Path string
}

// Watcher monitors a directory tree for new or modified .jsonl files
// using fsnotify.
type Watcher struct {
	rootDir string
	eventCh chan<- *LogFileEvent
	watcher *fsnotify.Watcher
	wg      sync.WaitGroup
	stopCh  chan struct{}
}

// NewWatcher creates a Watcher that monitors rootDir recursively and
// sends LogFileEvents to eventCh.
func NewWatcher(rootDir string, eventCh chan<- *LogFileEvent) *Watcher {
	return &Watcher{
		rootDir: rootDir,
		eventCh: eventCh,
		stopCh:  make(chan struct{}),
	}
}

// Start creates an fsnotify watcher, recursively adds all directories
// under rootDir, and starts the event loop goroutine.
func (w *Watcher) Start() error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = fsw

	// Recursively add all existing directories
	err = filepath.Walk(w.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}
		if info.IsDir() {
			return fsw.Add(path)
		}
		return nil
	})
	if err != nil {
		fsw.Close()
		return err
	}

	w.wg.Add(1)
	go w.eventLoop()

	return nil
}

// Stop closes the fsnotify watcher and waits for the event loop to finish.
func (w *Watcher) Stop() error {
	close(w.stopCh)
	if w.watcher != nil {
		w.watcher.Close()
	}
	w.wg.Wait()
	return nil
}

func (w *Watcher) eventLoop() {
	defer w.wg.Done()

	for {
		select {
		case <-w.stopCh:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			// Log errors in production; ignore in this implementation
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	path := event.Name

	// If a new directory is created, add it to the watch list
	if event.Has(fsnotify.Create) {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			// Recursively add the new directory and all subdirectories
			filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if fi.IsDir() {
					w.watcher.Add(p)
				}
				return nil
			})
			return
		}
	}

	// Only emit events for .jsonl files
	if !strings.HasSuffix(path, ".jsonl") {
		return
	}

	var evt *LogFileEvent

	switch {
	case event.Has(fsnotify.Create):
		evt = &LogFileEvent{
			Type: EventFileCreated,
			Path: path,
		}
	case event.Has(fsnotify.Write):
		evt = &LogFileEvent{
			Type: EventFileModified,
			Path: path,
		}
	case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
		evt = &LogFileEvent{
			Type: EventFileDeleted,
			Path: path,
		}
	default:
		return
	}

	// Non-blocking send
	select {
	case w.eventCh <- evt:
	default:
		// Channel full, drop event silently
	}
}
