package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/szaher/claude-monitor/internal/config"
	"github.com/szaher/claude-monitor/internal/db"
	"github.com/szaher/claude-monitor/internal/ingestion"
	"github.com/szaher/claude-monitor/internal/server"
)

// Serve starts the monitoring daemon and web UI.
func Serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 0, "HTTP port (overrides config)")
	host := fs.String("host", "", "HTTP host (overrides config)")
	noBrowser := fs.Bool("no-browser", false, "Don't open browser on start")
	_ = fs.Bool("daemon", false, "Run as background daemon (not yet implemented)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Load config
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}
	baseDir := filepath.Join(home, ".claude-monitor")
	configPath := filepath.Join(baseDir, "config.yaml")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Apply CLI overrides
	if *port != 0 {
		cfg.Server.Port = *port
	}
	if *host != "" {
		cfg.Server.Host = *host
	}

	// Init database
	dbPath := cfg.Storage.DatabasePath
	if dbPath == "" {
		dbPath = filepath.Join(baseDir, "claude-monitor.db")
	}

	database, err := db.InitDB(dbPath)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	defer database.Close()

	// Create event channel shared between receiver, watcher pipeline, and pipeline
	eventCh := make(chan []byte, 1000)

	// Create WebSocket hub
	hub := server.NewHub()
	defer hub.Close()

	// Start pipeline (batch processor)
	pipeline := ingestion.NewPipeline(database, hub.Broadcast, cfg.Cost.Models)
	pipeline.StartBatchProcessor(eventCh)

	// Start receiver (Unix socket)
	socketPath := filepath.Join(baseDir, "monitor.sock")
	receiver := ingestion.NewReceiver(socketPath, eventCh)
	if err := receiver.Start(); err != nil {
		log.Printf("Warning: could not start hook receiver: %v", err)
	} else {
		defer receiver.Stop()
	}

	// Start watcher (fsnotify on ~/.claude/projects/)
	claudeProjectsDir := filepath.Join(home, ".claude", "projects")
	logEventCh := make(chan *ingestion.LogFileEvent, 100)
	watcher := ingestion.NewWatcher(claudeProjectsDir, logEventCh)

	// Stop channel for the watcher bridge
	watcherStopCh := make(chan struct{})

	// Only start watcher if the directory exists
	if _, err := os.Stat(claudeProjectsDir); err == nil {
		if err := watcher.Start(); err != nil {
			log.Printf("Warning: could not start log watcher: %v", err)
		} else {
			defer watcher.Stop()
			// Start the bridge that reads file contents and feeds lines to the pipeline
			go startWatcherBridge(logEventCh, eventCh, watcherStopCh)
		}
	} else {
		log.Printf("Note: %s does not exist, log watcher not started", claudeProjectsDir)
	}

	// Start HTTP server
	srv := server.New(database, cfg, hub)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	// Open browser
	if !*noBrowser {
		go func() {
			// Wait a moment for the server to start
			time.Sleep(500 * time.Millisecond)
			url := fmt.Sprintf("http://%s", addr)
			if cfg.Server.Host == "0.0.0.0" || cfg.Server.Host == "" {
				url = fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
			}
			openBrowser(url)
		}()
	}

	// Wait for SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		httpServer.Shutdown(ctx)
	}()

	log.Printf("Claude Monitor starting on %s", addr)
	log.Printf("Database: %s", dbPath)

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}

	// Graceful shutdown: stop watcher bridge, then pipeline
	close(watcherStopCh)
	pipeline.Stop()

	log.Println("Claude Monitor stopped")
	return nil
}

// startWatcherBridge reads from the watcher's LogFileEvent channel, opens the
// modified/created files, reads new lines since the last offset, and sends each
// line as []byte to the pipeline's event channel.
func startWatcherBridge(logEventCh <-chan *ingestion.LogFileEvent, eventCh chan<- []byte, stopCh <-chan struct{}) {
	offsets := make(map[string]int64)

	for {
		select {
		case <-stopCh:
			return
		case event, ok := <-logEventCh:
			if !ok {
				return
			}
			if event.Type == ingestion.EventFileDeleted {
				delete(offsets, event.Path)
				continue
			}

			f, err := os.Open(event.Path)
			if err != nil {
				continue
			}

			offset := offsets[event.Path]
			if offset > 0 {
				f.Seek(offset, io.SeekStart)
			}

			scanner := bufio.NewScanner(f)
			buf := make([]byte, 0, 64*1024)
			scanner.Buffer(buf, 1024*1024)

			for scanner.Scan() {
				line := scanner.Bytes()
				if len(line) == 0 {
					continue
				}
				lineCopy := make([]byte, len(line))
				copy(lineCopy, line)

				select {
				case eventCh <- lineCopy:
				default:
					// Channel full, drop to avoid blocking
				}
			}

			// Update offset to current position
			pos, _ := f.Seek(0, io.SeekCurrent)
			offsets[event.Path] = pos
			f.Close()
		}
	}
}

// openBrowser opens the given URL in the user's default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}
