package cli

import (
	_ "embed"
	"fmt"
	"os/exec"
	"runtime"

	"fyne.io/systray"
)

//go:embed icon.png
var trayIcon []byte

type trayApp struct {
	addr      string
	onQuit    func()
	mStatus   *systray.MenuItem
	mSessions *systray.MenuItem
}

func newTrayApp(addr string, onQuit func()) *trayApp {
	return &trayApp{addr: addr, onQuit: onQuit}
}

func (t *trayApp) run() {
	systray.Run(t.onReady, t.onExit)
}

func (t *trayApp) onReady() {
	systray.SetIcon(trayIcon)
	systray.SetTitle("Claude Monitor")
	systray.SetTooltip("Claude Monitor — running")

	mOpen := systray.AddMenuItem("Open Dashboard", "Open web UI in browser")
	systray.AddSeparator()
	t.mStatus = systray.AddMenuItem("Running on "+t.addr, "")
	t.mStatus.Disable()
	t.mSessions = systray.AddMenuItem("Active sessions: ...", "")
	t.mSessions.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop Claude Monitor")

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				url := fmt.Sprintf("http://%s", t.addr)
				openInBrowser(url)
			case <-mQuit.ClickedCh:
				if t.onQuit != nil {
					t.onQuit()
				}
				systray.Quit()
				return
			}
		}
	}()
}

func (t *trayApp) onExit() {}

func (t *trayApp) stop() {
	systray.Quit()
}

func (t *trayApp) updateActiveSessions(count int) {
	if t.mSessions == nil {
		return
	}
	if count > 0 {
		t.mSessions.SetTitle(fmt.Sprintf("Active sessions: %d", count))
	} else {
		t.mSessions.SetTitle("No active sessions")
	}
}

func openInBrowser(url string) {
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
