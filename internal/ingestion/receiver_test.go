package ingestion

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

// shortSocketPath returns a socket path short enough for macOS
// (Unix socket paths are limited to 104 bytes on Darwin).
func shortSocketPath(t *testing.T, name string) string {
	t.Helper()
	path := fmt.Sprintf("/tmp/cm-test-%s-%d.sock", name, os.Getpid())
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func TestReceiver_AcceptConnections(t *testing.T) {
	socketPath := shortSocketPath(t, "accept")
	eventCh := make(chan []byte, 10)

	r := NewReceiver(socketPath, eventCh)
	if err := r.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer r.Stop()

	// Dial and send JSON
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	payload := `{"hook_event_name":"SessionStart","session_id":"sess-001"}`
	_, err = conn.Write([]byte(payload))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	conn.Close()

	// Wait for event to arrive on channel
	select {
	case data := <-eventCh:
		if string(data) != payload {
			t.Errorf("expected %q, got %q", payload, string(data))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}

	// Verify socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("socket file should exist while receiver is running")
	}
}

func TestReceiver_MultipleConnections(t *testing.T) {
	socketPath := shortSocketPath(t, "multi")
	eventCh := make(chan []byte, 10)

	r := NewReceiver(socketPath, eventCh)
	if err := r.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer r.Stop()

	// Send 3 events from separate connections
	payloads := []string{
		`{"hook_event_name":"SessionStart","session_id":"sess-001"}`,
		`{"hook_event_name":"PostToolUse","session_id":"sess-001","tool_name":"Read"}`,
		`{"hook_event_name":"Stop","session_id":"sess-001"}`,
	}

	for _, p := range payloads {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("Dial failed: %v", err)
		}
		_, err = conn.Write([]byte(p))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		conn.Close()
	}

	// Collect all 3 events
	received := make(map[string]bool)
	for i := 0; i < 3; i++ {
		select {
		case data := <-eventCh:
			received[string(data)] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for event %d", i+1)
		}
	}

	for _, p := range payloads {
		if !received[p] {
			t.Errorf("did not receive expected payload: %s", p)
		}
	}
}

func TestReceiver_Stop(t *testing.T) {
	socketPath := shortSocketPath(t, "stop")
	eventCh := make(chan []byte, 10)

	r := NewReceiver(socketPath, eventCh)
	if err := r.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := r.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Socket file should be removed after stop
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("socket file should be removed after Stop()")
	}

	// Dialing should fail after stop
	_, err := net.Dial("unix", socketPath)
	if err == nil {
		t.Error("expected dial to fail after Stop()")
	}
}

func TestReceiver_DropWhenChannelFull(t *testing.T) {
	socketPath := shortSocketPath(t, "drop")
	// Channel with capacity 1
	eventCh := make(chan []byte, 1)

	r := NewReceiver(socketPath, eventCh)
	if err := r.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer r.Stop()

	// Send 5 events rapidly without draining the channel
	for i := 0; i < 5; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("Dial failed: %v", err)
		}
		_, _ = conn.Write([]byte(`{"event":"test"}`))
		conn.Close()
	}

	// Give time for processing
	time.Sleep(500 * time.Millisecond)

	// We should get at least 1, but some should have been dropped
	// (channel capacity is 1, so at most 1 can be buffered)
	count := 0
	for {
		select {
		case <-eventCh:
			count++
		default:
			goto done
		}
	}
done:
	if count == 0 {
		t.Error("expected at least 1 event to be received")
	}
	// We just verify it doesn't panic or block -- some events are dropped
}
