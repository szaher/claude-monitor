package ingestion

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcher_DetectNewFile(t *testing.T) {
	rootDir := t.TempDir()
	eventCh := make(chan *LogFileEvent, 10)

	w := NewWatcher(rootDir, eventCh)
	if err := w.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer w.Stop()

	// Give the watcher time to set up
	time.Sleep(100 * time.Millisecond)

	// Create a .jsonl file
	filePath := filepath.Join(rootDir, "session.jsonl")
	if err := os.WriteFile(filePath, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Wait for the event
	select {
	case evt := <-eventCh:
		if evt.Type != EventFileCreated && evt.Type != EventFileModified {
			t.Errorf("expected EventFileCreated or EventFileModified, got %v", evt.Type)
		}
		if evt.Path != filePath {
			t.Errorf("expected path %q, got %q", filePath, evt.Path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for file creation event")
	}
}

func TestWatcher_DetectFileModification(t *testing.T) {
	rootDir := t.TempDir()
	eventCh := make(chan *LogFileEvent, 10)

	// Create the file before starting the watcher
	filePath := filepath.Join(rootDir, "session.jsonl")
	if err := os.WriteFile(filePath, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	w := NewWatcher(rootDir, eventCh)
	if err := w.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer w.Stop()

	// Give the watcher time to set up
	time.Sleep(100 * time.Millisecond)

	// Append to the file
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	_, err = f.WriteString(`{"type":"assistant"}` + "\n")
	if err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}
	f.Close()

	// Wait for the modification event
	select {
	case evt := <-eventCh:
		if evt.Type != EventFileModified {
			t.Errorf("expected EventFileModified, got %v", evt.Type)
		}
		if evt.Path != filePath {
			t.Errorf("expected path %q, got %q", filePath, evt.Path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for file modification event")
	}
}

func TestWatcher_IgnoreNonJSONL(t *testing.T) {
	rootDir := t.TempDir()
	eventCh := make(chan *LogFileEvent, 10)

	w := NewWatcher(rootDir, eventCh)
	if err := w.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer w.Stop()

	// Give the watcher time to set up
	time.Sleep(100 * time.Millisecond)

	// Create a non-jsonl file
	txtPath := filepath.Join(rootDir, "notes.txt")
	if err := os.WriteFile(txtPath, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Should not receive any event for .txt files
	select {
	case evt := <-eventCh:
		t.Errorf("should not have received event for .txt file, got: %+v", evt)
	case <-time.After(500 * time.Millisecond):
		// Expected: no event
	}
}

func TestWatcher_DetectNewSubdirectory(t *testing.T) {
	rootDir := t.TempDir()
	eventCh := make(chan *LogFileEvent, 10)

	w := NewWatcher(rootDir, eventCh)
	if err := w.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer w.Stop()

	// Give the watcher time to set up
	time.Sleep(100 * time.Millisecond)

	// Create a subdirectory and a .jsonl file in it
	subDir := filepath.Join(rootDir, "project1")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Give the watcher time to add the new directory
	time.Sleep(200 * time.Millisecond)

	filePath := filepath.Join(subDir, "session.jsonl")
	if err := os.WriteFile(filePath, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Wait for the file creation event
	select {
	case evt := <-eventCh:
		if evt.Path != filePath {
			t.Errorf("expected path %q, got %q", filePath, evt.Path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event in new subdirectory")
	}
}

func TestWatcher_Stop(t *testing.T) {
	rootDir := t.TempDir()
	eventCh := make(chan *LogFileEvent, 10)

	w := NewWatcher(rootDir, eventCh)
	if err := w.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := w.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// After stop, creating a file should not trigger events
	filePath := filepath.Join(rootDir, "after-stop.jsonl")
	if err := os.WriteFile(filePath, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	select {
	case evt := <-eventCh:
		t.Errorf("should not receive events after Stop(), got: %+v", evt)
	case <-time.After(500 * time.Millisecond):
		// Expected: no event
	}
}
