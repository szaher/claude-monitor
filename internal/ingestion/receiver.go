package ingestion

import (
	"io"
	"net"
	"os"
	"sync"
)

// Receiver is a Unix socket server that accepts connections from hook scripts
// and forwards their JSON payloads to an event channel.
type Receiver struct {
	socketPath string
	eventCh    chan<- []byte
	listener   net.Listener
	wg         sync.WaitGroup
	stopCh     chan struct{}
}

// NewReceiver creates a new Receiver that listens on the given Unix socket path
// and sends received data to eventCh.
func NewReceiver(socketPath string, eventCh chan<- []byte) *Receiver {
	return &Receiver{
		socketPath: socketPath,
		eventCh:    eventCh,
		stopCh:     make(chan struct{}),
	}
}

// Start removes any old socket file, creates a new Unix socket listener,
// sets permissions to 0666, and starts the accept loop in a goroutine.
func (r *Receiver) Start() error {
	// Remove old socket if it exists
	os.Remove(r.socketPath)

	ln, err := net.Listen("unix", r.socketPath)
	if err != nil {
		return err
	}
	r.listener = ln

	// Make socket world-writable so hook scripts can connect
	if err := os.Chmod(r.socketPath, 0666); err != nil {
		ln.Close()
		os.Remove(r.socketPath)
		return err
	}

	r.wg.Add(1)
	go r.acceptLoop()

	return nil
}

// Stop closes the listener, waits for all connection goroutines to finish,
// and removes the socket file.
func (r *Receiver) Stop() error {
	close(r.stopCh)

	if r.listener != nil {
		r.listener.Close()
	}

	r.wg.Wait()

	os.Remove(r.socketPath)
	return nil
}

func (r *Receiver) acceptLoop() {
	defer r.wg.Done()

	for {
		conn, err := r.listener.Accept()
		if err != nil {
			// Check if we're shutting down
			select {
			case <-r.stopCh:
				return
			default:
			}
			// Transient error, continue
			continue
		}

		r.wg.Add(1)
		go r.handleConnection(conn)
	}
}

func (r *Receiver) handleConnection(conn net.Conn) {
	defer r.wg.Done()
	defer conn.Close()

	data, err := io.ReadAll(conn)
	if err != nil {
		return
	}

	if len(data) == 0 {
		return
	}

	// Non-blocking send: drop event if channel is full
	select {
	case r.eventCh <- data:
	default:
		// Channel full, drop event silently
	}
}
