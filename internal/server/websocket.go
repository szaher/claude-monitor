package server

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Hub manages a set of active WebSocket connections and broadcasts
// messages to all of them.
type Hub struct {
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
}

// NewHub creates a new Hub with an empty client set.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]bool),
	}
}

// HandleWebSocket upgrades an HTTP connection to WebSocket and registers
// the connection with the hub. It runs a read loop that waits for the
// client to disconnect, then removes the connection.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	// Read loop: wait for client to close
	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// Broadcast sends data to all connected WebSocket clients.
// Connections that fail to receive the message are removed.
func (h *Hub) Broadcast(data []byte) {
	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for conn := range h.clients {
		clients = append(clients, conn)
	}
	h.mu.RUnlock()

	var failed []*websocket.Conn
	for _, conn := range clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			failed = append(failed, conn)
		}
	}

	if len(failed) > 0 {
		h.mu.Lock()
		for _, conn := range failed {
			delete(h.clients, conn)
			conn.Close()
		}
		h.mu.Unlock()
	}
}

// Close closes all connected WebSocket clients.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for conn := range h.clients {
		conn.Close()
		delete(h.clients, conn)
	}
}
