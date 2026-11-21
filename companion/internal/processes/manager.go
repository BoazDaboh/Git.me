package processes

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitme/companion/internal/workspace"
)

type Config struct {
	ProjectID  string `json:"project_id"`
	ServiceID  string `json:"service_id"`
	Name       string `json:"name"`
	Folder     string `json:"folder"`
	Command    string `json:"command"`
	Port       *int   `json:"port"`
	LocalURL   string `json:"local_url"`
	HealthPath string `json:"health_path"`
}
type Plan struct {
	Config
	Token       string    `json:"token"`
	Action      string    `json:"action"`
	ExpiresAt   time.Time `json:"expires_at"`
	fingerprint string
	runID       string
}
type Run struct {
	Config
	ID        string     `json:"run_id"`
	State     string     `json:"state"`
	PID       int        `json:"pid"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	ExitCode  *int       `json:"exit_code"`
	Error     string     `json:"error"`
	Logs      string     `json:"logs,omitempty"`
	Truncated bool       `json:"truncated"`
}
type record struct {
	run    Run
	child  *child
	done   chan struct{}
	output *logBuffer
}
type Manager struct {
	store     *workspace.Store
	mu        sync.Mutex
	operation sync.Mutex
	plans     map[string]Plan
	runs      map[string]*record
	closed    bool
	protected []string
}

func New(store *workspace.Store) *Manager {
	m := &Manager{store: store, plans: map[string]Plan{}, runs: map[string]*record{}}
	app := os.Getenv("GITME_APP_DIR")
	if app == "" {
		if cwd, err := os.Getwd(); err == nil {
			if info, err := os.Stat(filepath.Join(cwd, "cmd", "gitme", "main.go")); err == nil && info.Mode().IsRegular() {
				app = filepath.Dir(cwd)
			}
		}
	}
	if app != "" {
		m.protected = []string{filepath.Clean(app), filepath.Join(app, "companion")}
	}
	return m
}
func key(project, service string) string { return project + "\x00" + service }
func identifier() string {
	data := make([]byte, 24)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data)
}
func fingerprint(config Config) string {
	raw, _ := json.Marshal(config)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func active(state string) bool { return state == "running" || state == "stopping" }

func (m *Manager) config(ctx context.Context, projectID, serviceID string) (Config, error) {
	if projectID == "" || serviceID == "" {
		return Config{}, errors.New("choose a saved project and service")
	}
	snapshot, err := m.store.Load(ctx)
	if err != nil {
		return Config{}, errors.New("could not read saved service")
	}
	var data struct {
		Projects []struct {
			ID       string `json:"id"`
			Services []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Folder     string `json:"folder"`
				Port       *int   `json:"port"`
				HealthPath string `json:"healthPath"`
				Commands   struct {
					Develop string `json:"develop"`
				} `json:"commands"`
				Environments struct {
					Local struct {
						URL string `json:"url"`
					} `json:"local"`
				} `json:"environments"`
			} `json:"services"`
		} `json:"projects"`
	}
	if json.Unmarshal(snapshot.Workspace, &data) != nil {
		return Config{}, errors.New("save the workspace first")
	}
	for _, project := range data.Projects {
		if project.ID != projectID {
			continue
		}
		for _, service := range project.Services {
			if service.ID == serviceID {
				return Config{ProjectID: projectID, ServiceID: serviceID, Name: service.Name, Folder: service.Folder, Command: service.Commands.Develop, Port: service.Port, LocalURL: service.Environments.Local.URL, HealthPath: service.HealthPath}, nil
			}
		}
	}
	return Config{}, errors.New("saved service not found")
}

func (m *Manager) validate(config Config) (Config, error) {
	if config.Command == "" || len(config.Command) > 4000 || strings.ContainsAny(config.Command, "\x00\r\n") {
		return config, errors.New("set a single-line development command in the saved profile")
	}
	if !filepath.IsAbs(config.Folder) || strings.HasPrefix(config.Folder, `\\`) || strings.HasPrefix(config.Folder, "//") {
		return config, errors.New("choose an absolute local service folder")
	}
	folder, err := filepath.EvalSymlinks(filepath.Clean(config.Folder))
	if err != nil {
		return config, errors.New("service folder does not exist")
	}
	info, err := os.Stat(folder)
	if err != nil || !info.IsDir() || filepath.Dir(folder) == folder || strings.HasPrefix(folder, `\\`) {
		return config, errors.New("choose a local project folder, not a drive root")
	}
	config.Folder = folder
	for _, path := range m.protected {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil && strings.EqualFold(resolved, folder) {
			return config, errors.New("keep Git/Me's own frontend and companion in their startup terminals")
		}
	}
	if config.Port != nil && (*config.Port < 1 || *config.Port > 65535 || *config.Port == 3015 || *config.Port == 47821) {
		return config, errors.New("choose a valid development port; 3015 and 47821 are reserved for Git/Me")
	}
	return config, nil
}

func portFree(port *int) error {
	if port == nil {
		return nil
	}
	for _, host := range []string{"127.0.0.1", "::1"} {
		listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(*port)))
		if err != nil {
			if host == "::1" {
				var op *net.OpError
				if errors.As(err, &op) && strings.Contains(err.Error(), "not supported") {
					continue
				}
			}
			return fmt.Errorf("port %d is already in use or unavailable; stop its existing server or choose another port", *port)
		}
		listener.Close()
	}
	return nil
}

func (m *Manager) Prepare(ctx context.Context, project, service, action string) (Plan, error) {
	if action != "start" && action != "restart" {
		return Plan{}, errors.New("choose start or restart")
	}
	config, err := m.config(ctx, project, service)
	if err != nil {
		return Plan{}, err
	}
	config, err = m.validate(config)
	if err != nil {
		return Plan{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Plan{}, errors.New("companion is shutting down")
	}
	owner := ""
	if old := m.runs[key(project, service)]; old != nil && active(old.run.State) {
		owner = old.run.ID
	}
	if action == "start" && owner != "" {
		return Plan{}, errors.New("this service is already running in Git/Me")
	}
	if action == "restart" && owner == "" {
		return Plan{}, errors.New("there is no Git/Me-owned process to restart; use Start")
	}
	if action == "start" {
		if err := portFree(config.Port); err != nil {
			return Plan{}, err
		}
	}
	for token, plan := range m.plans {
		if time.Now().After(plan.ExpiresAt) {
			delete(m.plans, token)
		}
	}
	if len(m.plans) >= 64 {
		return Plan{}, errors.New("too many pending reviews; wait two minutes and retry")
	}
	plan := Plan{Config: config, Token: identifier(), Action: action, ExpiresAt: time.Now().Add(2 * time.Minute), fingerprint: fingerprint(config), runID: owner}
	m.plans[plan.Token] = plan
	return plan, nil
}

func (m *Manager) Start(ctx context.Context, token string) (Run, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	m.mu.Lock()
	plan, ok := m.plans[token]
	delete(m.plans, token)
	closed := m.closed
	m.mu.Unlock()
	if closed || !ok || time.Now().After(plan.ExpiresAt) {
		return Run{}, errors.New("launch review expired or was already used; review the command again")
	}
	config, err := m.config(ctx, plan.ProjectID, plan.ServiceID)
	if err != nil {
		return Run{}, err
	}
	config, err = m.validate(config)
	if err != nil {
		return Run{}, err
	}
	if fingerprint(config) != plan.fingerprint {
		return Run{}, errors.New("service settings changed after review; review again")
	}
	m.mu.Lock()
	old := m.runs[key(config.ProjectID, config.ServiceID)]
	owner := ""
	running := 0
	for _, r := range m.runs {
		if active(r.run.State) {
			running++
		}
	}
	if old != nil && active(old.run.State) {
		owner = old.run.ID
	}
	m.mu.Unlock()
	if owner != plan.runID {
		return Run{}, errors.New("process state changed after review; review again")
	}
	if plan.Action == "restart" {
		if err := m.stop(old); err != nil {
			return Run{}, err
		}
	}
	if running >= 8 && plan.Action != "restart" {
		return Run{}, errors.New("eight services are already running; stop one first")
	}
	if err := portFree(config.Port); err != nil {
		return Run{}, err
	}
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	record := &record{run: Run{Config: config, ID: identifier(), State: "running", StartedAt: time.Now()}, done: make(chan struct{}), output: &logBuffer{}}
	process, err := launch(config.Folder, config.Command, record.output)
	if err != nil {
		return Run{}, fmt.Errorf("could not launch command: %w", err)
	}
	record.child = process
	record.run.PID = process.pid()
	m.mu.Lock()
	m.runs[key(config.ProjectID, config.ServiceID)] = record
	if len(m.runs) > 64 {
		for k, r := range m.runs {
			if r != record && !active(r.run.State) {
				delete(m.runs, k)
				break
			}
		}
	}
	m.mu.Unlock()
	go func() {
		err := process.wait()
		process.close()
		m.mu.Lock()
		end := time.Now()
		record.run.EndedAt = &end
		code := process.exitCode()
		record.run.ExitCode = &code
		if record.run.State == "stopping" {
			record.run.State = "stopped"
		} else if err != nil {
			record.run.State = "failed"
			record.run.Error = err.Error()
		} else {
			record.run.State = "exited"
		}
		close(record.done)
		m.mu.Unlock()
	}()
	return m.Status(config.ProjectID, config.ServiceID), nil
}

func (m *Manager) stop(record *record) error {
	if record == nil {
		return errors.New("no owned process found")
	}
	m.mu.Lock()
	if !active(record.run.State) {
		m.mu.Unlock()
		return errors.New("this process has already exited")
	}
	previous := record.run.State
	record.run.State = "stopping"
	m.mu.Unlock()
	if err := record.child.kill(); err != nil {
		m.mu.Lock()
		if record.run.EndedAt == nil {
			record.run.State = previous
		}
		m.mu.Unlock()
		return err
	}
	select {
	case <-record.done:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("stop is still pending; check the process status before retrying")
	}
}

func (m *Manager) Stop(project, service, runID string) (Run, error) {
	m.operation.Lock()
	defer m.operation.Unlock()
	m.mu.Lock()
	r := m.runs[key(project, service)]
	valid := r != nil && r.run.ID == runID && runID != ""
	m.mu.Unlock()
	if !valid {
		return Run{}, errors.New("process ownership changed; refresh the service before stopping it")
	}
	if err := m.stop(r); err != nil {
		return Run{}, err
	}
	return m.Status(project, service), nil
}

func (m *Manager) Status(project, service string) Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.runs[key(project, service)]
	if r == nil {
		return Run{Config: Config{ProjectID: project, ServiceID: service}, State: "not_managed"}
	}
	result := r.run
	result.Logs, result.Truncated = r.output.snapshot()
	return result
}
func (m *Manager) List() []Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []Run{}
	for _, record := range m.runs {
		run := record.run
		result = append(result, run)
	}
	return result
}
func (m *Manager) Close() {
	m.operation.Lock()
	defer m.operation.Unlock()
	m.mu.Lock()
	m.closed = true
	owned := []*record{}
	for _, r := range m.runs {
		if active(r.run.State) {
			r.run.State = "stopping"
			owned = append(owned, r)
		}
	}
	m.mu.Unlock()
	for _, r := range owned {
		_ = r.child.kill()
	}
	deadline := time.After(3 * time.Second)
	for _, r := range owned {
		select {
		case <-r.done:
		case <-deadline:
			return
		}
	}
}

type logBuffer struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (b *logBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	count := len(data)
	b.data = append(b.data, data...)
	if len(b.data) > 65536 {
		b.data = append([]byte{}, b.data[len(b.data)-65536:]...)
		b.truncated = true
	}
	return count, nil
}

var ansi = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)

func (b *logBuffer) snapshot() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := ansi.ReplaceAllString(strings.ToValidUTF8(string(b.data), "�"), "")
	text = strings.Map(func(r rune) rune {
		if r == '\r' {
			return '\n'
		}
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
	return text, b.truncated
}
