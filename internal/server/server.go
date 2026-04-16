package server

import (
	"database/sql"
	"net/http"

	"github.com/szaher/claude-monitor/internal/config"
	"github.com/szaher/claude-monitor/web"
)

// Server is the HTTP server for claude-monitor. It provides REST API
// endpoints, a WebSocket endpoint for real-time updates, and serves
// the embedded static web UI.
type Server struct {
	db     *sql.DB
	config *config.Config
	hub    *Hub
	mux    *http.ServeMux
}

// New creates a new Server with the given database, config, and WebSocket hub.
func New(db *sql.DB, cfg *config.Config, hub *Hub) *Server {
	s := &Server{
		db:     db,
		config: cfg,
		hub:    hub,
		mux:    http.NewServeMux(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// API routes
	s.mux.HandleFunc("/api/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/sessions/", s.handleSessionDetail)
	s.mux.HandleFunc("/api/messages", s.handleMessages)
	s.mux.HandleFunc("/api/tool-calls", s.handleToolCalls)
	s.mux.HandleFunc("/api/subagents", s.handleSubagents)
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.HandleFunc("/api/stats/daily", s.handleDailyStats)
	s.mux.HandleFunc("/api/stats/tools", s.handleToolStats)
	s.mux.HandleFunc("/api/stats/models", s.handleModelStats)
	s.mux.HandleFunc("/api/stats/projects", s.handleProjectStats)
	s.mux.HandleFunc("/api/stats/skills", s.handleSkillStats)
	s.mux.HandleFunc("/api/stats/mcp", s.handleMCPStats)
	s.mux.HandleFunc("/api/stats/errors", s.handleErrors)
	s.mux.HandleFunc("/api/stats/session-breakdown", s.handleSessionBreakdown)
	s.mux.HandleFunc("/api/stats/project-breakdown", s.handleProjectBreakdown)
	s.mux.HandleFunc("/api/stats/token-efficiency", s.handleTokenEfficiency)
	s.mux.HandleFunc("/api/stats/prompt-patterns", s.handlePromptPatterns)
	s.mux.HandleFunc("/api/stats/file-heatmap", s.handleFileHeatmap)
	s.mux.HandleFunc("/api/budgets/status", s.budgetStatus)
	s.mux.HandleFunc("/api/budgets/", s.handleBudgetDetail)
	s.mux.HandleFunc("/api/budgets", s.handleBudgets)
	s.mux.HandleFunc("/api/tags", s.handleTags)
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/search", s.handleSearch)
	s.mux.HandleFunc("/api/export", s.handleExport)
	s.mux.HandleFunc("/ws", s.hub.HandleWebSocket)

	// Static files (embedded)
	s.mux.Handle("/", http.FileServer(http.FS(web.StaticFiles())))
}

// Handler returns the http.Handler for this server.
func (s *Server) Handler() http.Handler {
	return s.mux
}
