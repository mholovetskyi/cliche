// Package devserver runs a project's dev server (npm run dev — Vite, Next, CRA,
// …) and exposes its live URL so Cliché Studio can show a real, hot-reloading
// preview: build a React app and watch it run, edit and see it update — the
// Lovable experience. It is user-initiated (like Deploy), and bounded — one
// server at a time, its whole process tree killed on stop, session switch, or
// shutdown. Zero third-party dependencies: os/exec + the standard library.
package devserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// States the dev server can be in (mirrored to the UI).
const (
	StateStopped    = "stopped"
	StateInstalling = "installing"
	StateStarting   = "starting"
	StateRunning    = "running"
	StateError      = "error"
)

const maxLogLines = 240

// devURLRe matches the local URL a dev server prints once it's listening.
var devURLRe = regexp.MustCompile(`https?://(?:localhost|127\.0\.0\.1|0\.0\.0\.0)(?::\d+)?`)

// Status is the snapshot the UI polls.
type Status struct {
	State    string   `json:"state"`
	URL      string   `json:"url"`
	Dir      string   `json:"dir"`
	Detected bool     `json:"detected"` // a runnable dev script exists under the project
	Script   string   `json:"script"`   // e.g. "npm run dev"
	Logs     []string `json:"logs"`
}

// Manager owns at most one running dev server.
type Manager struct {
	mu     sync.Mutex
	state  string
	url    string
	dir    string
	cmd    *exec.Cmd
	cancel context.CancelFunc
	logs   []string
}

// New returns an idle manager.
func New() *Manager { return &Manager{state: StateStopped} }

// DetectIn looks for a runnable dev script in root, or one level down (an app
// scaffolded into a subfolder), and returns the app dir + the command to run.
func DetectIn(root string) (appDir, script string, ok bool) {
	if root == "" {
		root = "."
	}
	if s, found := devScript(root); found {
		return root, s, true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", "", false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if n := e.Name(); strings.HasPrefix(n, ".") || n == "node_modules" {
			continue
		}
		sub := filepath.Join(root, e.Name())
		if s, found := devScript(sub); found {
			return sub, s, true
		}
	}
	return "", "", false
}

// devScript returns the preferred dev command from dir's package.json, if any.
func devScript(dir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "", false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return "", false
	}
	for _, name := range []string{"dev", "start", "serve"} {
		if _, has := pkg.Scripts[name]; has {
			return "npm run " + name, true
		}
	}
	return "", false
}

// Status returns the current state plus whether a dev script is detectable under
// root (so the UI can offer "Run app" even while stopped).
func (m *Manager) Status(root string) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, script, ok := DetectIn(root)
	logs := make([]string, len(m.logs))
	copy(logs, m.logs)
	return Status{State: m.state, URL: m.url, Dir: m.dir, Detected: ok, Script: script, Logs: logs}
}

// Running reports whether a server is up, and its URL.
func (m *Manager) Running() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.url, m.state == StateRunning
}

// Start launches the dev server for the app under root (installing deps first if
// node_modules is missing). A no-op if one is already coming up or running.
func (m *Manager) Start(root string) error {
	m.mu.Lock()
	if m.state == StateRunning || m.state == StateStarting || m.state == StateInstalling {
		m.mu.Unlock()
		return nil
	}
	appDir, script, ok := DetectIn(root)
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("no package.json with a dev/start/serve script found — build an app first")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("npm not found on PATH — install Node.js to run a dev server")
	}
	needInstall := !dirExists(filepath.Join(appDir, "node_modules"))
	m.dir, m.url, m.logs = appDir, "", nil
	if needInstall {
		m.state = StateInstalling
	} else {
		m.state = StateStarting
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.mu.Unlock()

	go m.run(ctx, appDir, script, needInstall)
	return nil
}

// Stop kills the server's whole process tree and resets to idle.
func (m *Manager) Stop() {
	m.mu.Lock()
	cancel, cmd := m.cancel, m.cmd
	m.state, m.url, m.cmd, m.cancel = StateStopped, "", nil, nil
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		killTree(cmd.Process.Pid)
	}
	if cancel != nil {
		cancel()
	}
}

// Restart stops then starts.
func (m *Manager) Restart(root string) error {
	m.Stop()
	return m.Start(root)
}

func (m *Manager) run(ctx context.Context, appDir, script string, needInstall bool) {
	if needInstall {
		m.log("$ npm install")
		install := exec.CommandContext(ctx, "npm", "install", "--no-audit", "--no-fund")
		install.Dir = appDir
		setProcGroup(install)
		m.pipe(install)
		if err := install.Run(); err != nil {
			m.fail("npm install failed: " + err.Error())
			return
		}
	}

	m.setState(StateStarting)
	m.log("$ " + script)
	parts := strings.Fields(script) // npm run <name>
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = appDir
	setProcGroup(cmd)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		m.fail("could not start the dev server: " + err.Error())
		return
	}
	m.mu.Lock()
	m.cmd = cmd
	m.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); m.scan(stdout) }()
	go func() { defer wg.Done(); m.scan(stderr) }()
	wg.Wait()
	_ = cmd.Wait()

	// The process exited. If we weren't explicitly stopped, mark it stopped.
	m.mu.Lock()
	if m.state != StateStopped {
		m.state = StateStopped
		m.url = ""
	}
	m.mu.Unlock()
}

// scan reads a stream line by line, logging each and catching the dev URL — once
// a localhost URL appears the server is live.
func (m *Manager) scan(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for sc.Scan() {
		line := stripANSI(sc.Text())
		m.log(line)
		m.mu.Lock()
		if m.url == "" {
			if u := devURLRe.FindString(line); u != "" {
				m.url = normalizeURL(u)
				if m.state != StateStopped {
					m.state = StateRunning
				}
			}
		}
		m.mu.Unlock()
	}
}

func (m *Manager) pipe(cmd *exec.Cmd) {
	out, _ := cmd.StdoutPipe()
	errp, _ := cmd.StderrPipe()
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			m.log(stripANSI(sc.Text()))
		}
	}()
	go func() {
		sc := bufio.NewScanner(errp)
		for sc.Scan() {
			m.log(stripANSI(sc.Text()))
		}
	}()
}

func (m *Manager) log(line string) {
	if line == "" {
		return
	}
	m.mu.Lock()
	m.logs = append(m.logs, line)
	if len(m.logs) > maxLogLines {
		m.logs = m.logs[len(m.logs)-maxLogLines:]
	}
	m.mu.Unlock()
}

func (m *Manager) setState(s string) {
	m.mu.Lock()
	if m.state != StateStopped {
		m.state = s
	}
	m.mu.Unlock()
}

func (m *Manager) fail(msg string) {
	m.log(msg)
	m.mu.Lock()
	if m.state != StateStopped {
		m.state = StateError
	}
	m.mu.Unlock()
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func normalizeURL(u string) string {
	u = strings.TrimRight(u, "/")
	u = strings.Replace(u, "0.0.0.0", "localhost", 1)
	return u
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string { return strings.TrimSpace(ansiRe.ReplaceAllString(s, "")) }
