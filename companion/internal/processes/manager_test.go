package processes

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitme/companion/internal/workspace"
)

func testManager(t *testing.T, command string, port *int) (*Manager, *workspace.Store, string) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "project with spaces")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	manager := New(store)
	t.Cleanup(func() { manager.Close(); store.Close() })
	profile := map[string]any{"id": "s1", "name": "Fixture", "role": "backend", "stack": []string{"Node.js"}, "runtime": "Node.js", "folder": directory, "repository": "", "branch": "main", "port": port, "healthPath": "/health", "commands": map[string]string{"develop": command, "install": "", "build": "", "test": "", "start": ""}, "deployment": map[string]string{"provider": "", "method": "native", "build": "", "start": "", "dockerfile": "", "image": ""}, "environments": map[string]any{"local": map[string]any{"url": "", "variables": []string{}}, "preview": map[string]any{"url": "", "variables": []string{}}, "production": map[string]any{"url": "", "variables": []string{}}}, "updatedAt": 1}
	project := map[string]any{"id": "p1", "name": "Fixture", "initials": "f", "color": "blue", "description": "", "stack": []string{}, "stage": "Building", "progress": 0, "branch": "main", "folder": directory, "url": "", "local": "", "note": "", "services": []any{profile}}
	data := map[string]any{"version": 1, "projects": []any{project}, "tasks": []any{}, "notes": []any{}, "sessions": []any{}, "drafts": []any{}, "focus": nil, "document": map[string]string{"title": "", "text": ""}}
	raw, _ := json.Marshal(data)
	snapshot, _ := store.Load(context.Background())
	if _, err := store.Save(context.Background(), snapshot.DatabaseID, 0, raw); err != nil {
		t.Fatal(err)
	}
	return manager, store, directory
}
func modify(t *testing.T, store *workspace.Store, update func(map[string]any)) {
	t.Helper()
	snapshot, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err = json.Unmarshal(snapshot.Workspace, &data); err != nil {
		t.Fatal(err)
	}
	profile := data["projects"].([]any)[0].(map[string]any)["services"].([]any)[0].(map[string]any)
	update(profile)
	raw, _ := json.Marshal(data)
	if _, err = store.Save(context.Background(), snapshot.DatabaseID, snapshot.Revision, raw); err != nil {
		t.Fatal(err)
	}
}
func availablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	until := time.Now().Add(8 * time.Second)
	for time.Now().Before(until) {
		if condition() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

func TestReviewOwnershipRestartAndChildCleanup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process controls")
	}
	node, err := exec.LookPath("node.exe")
	if err != nil {
		t.Skip("Node required for process tree integration")
	}
	port := availablePort(t)
	m, store, folder := testManager(t, "placeholder", &port)
	source := `const http=require('node:http'),{spawn}=require('node:child_process');if(process.argv[2]==='child'){http.createServer((req,res)=>res.end('ok')).listen(Number(process.argv[3]),'127.0.0.1',()=>console.log('CHILD_READY'));setInterval(()=>console.log('child alive'),250)}else{spawn(process.execPath,[__filename,'child',process.argv[2]],{stdio:'inherit'});console.log('PARENT_READY');setInterval(()=>{},1000)}`
	if err = os.WriteFile(filepath.Join(folder, "server.cjs"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	command := `"` + node + `" "` + filepath.Join(folder, "server.cjs") + `" ` + strconv.Itoa(port)
	modify(t, store, func(profile map[string]any) { profile["commands"].(map[string]any)["develop"] = command })
	plan, err := m.Prepare(context.Background(), "p1", "s1", "start")
	if err != nil {
		t.Fatal(err)
	}
	if m.Status("p1", "s1").State != "not_managed" {
		t.Fatal("prepare executed command")
	}
	run, err := m.Start(context.Background(), plan.Token)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return strings.Contains(m.Status("p1", "s1").Logs, "CHILD_READY") })
	if _, err = m.Start(context.Background(), plan.Token); err == nil {
		t.Fatal("reused token")
	}
	if _, err = m.Prepare(context.Background(), "p1", "s1", "start"); err == nil {
		t.Fatal("duplicate start accepted")
	}
	health, err := m.Health(context.Background(), "p1", "s1")
	if err != nil || !health.Responding || health.Status != 200 {
		t.Fatalf("health=%+v %v", health, err)
	}
	restart, err := m.Prepare(context.Background(), "p1", "s1", "restart")
	if err != nil {
		t.Fatal(err)
	}
	next, err := m.Start(context.Background(), restart.Token)
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == next.ID {
		t.Fatal("restart reused run identity")
	}
	waitFor(t, func() bool { return strings.Contains(m.Status("p1", "s1").Logs, "CHILD_READY") })
	if _, err = m.Stop("p1", "s1", run.ID); err == nil {
		t.Fatal("stale run could stop new process")
	}
	if m.Status("p1", "s1").State != "running" {
		t.Fatal("stale stop affected new process")
	}
	stopped, err := m.Stop("p1", "s1", next.ID)
	if err != nil || stopped.State != "stopped" {
		t.Fatalf("stop=%+v %v", stopped, err)
	}
	if err = portFree(&port); err != nil {
		t.Fatal("child server survived stop: " + err.Error())
	}
	plan, err = m.Prepare(context.Background(), "p1", "s1", "start")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start(context.Background(), plan.Token); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return strings.Contains(m.Status("p1", "s1").Logs, "CHILD_READY") })
	m.Close()
	if err = portFree(&port); err != nil {
		t.Fatal("child survived companion shutdown")
	}
}

func TestReviewInvalidationAndOccupiedPorts(t *testing.T) {
	m, store, _ := testManager(t, "echo reviewed", nil)
	plan, err := m.Prepare(context.Background(), "p1", "s1", "start")
	if err != nil {
		t.Fatal(err)
	}
	modify(t, store, func(profile map[string]any) { profile["commands"].(map[string]any)["develop"] = "echo changed" })
	if _, err = m.Start(context.Background(), plan.Token); err == nil {
		t.Fatal("changed command executed under old review")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	modify(t, store, func(profile map[string]any) { profile["port"] = port })
	if _, err = m.Prepare(context.Background(), "p1", "s1", "start"); err == nil {
		t.Fatal("occupied port accepted")
	}
	if _, err = m.Stop("p1", "s1", "external-pid"); err == nil {
		t.Fatal("external process adopted")
	}
	for _, reserved := range []int{3015, 47821} {
		modify(t, store, func(profile map[string]any) { profile["port"] = reserved })
		if _, err = m.Prepare(context.Background(), "p1", "s1", "start"); err == nil {
			t.Fatal("reserved port accepted")
		}
	}
}

func TestExitAndNpmBatchCommand(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process controls")
	}
	m, _, folder := testManager(t, "npm.cmd run dev", nil)
	if _, err := exec.LookPath("npm.cmd"); err != nil {
		t.Skip("npm required")
	}
	if err := os.WriteFile(filepath.Join(folder, "package.json"), []byte(`{"scripts":{"dev":"node -e \"console.log('batch-ok');process.exit(7)\""}}`), 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := m.Prepare(context.Background(), "p1", "s1", "start")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start(context.Background(), plan.Token); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return m.Status("p1", "s1").State == "failed" })
	run := m.Status("p1", "s1")
	if run.ExitCode == nil || *run.ExitCode != 7 || !strings.Contains(run.Logs, "batch-ok") {
		t.Fatalf("exit=%+v", run)
	}
}

func TestHealthDoesNotFollowRemoteRedirects(t *testing.T) {
	m, store, _ := testManager(t, "echo unused", nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "https://example.com/private", 302) }))
	defer server.Close()
	modify(t, store, func(profile map[string]any) {
		profile["environments"].(map[string]any)["local"].(map[string]any)["url"] = server.URL
	})
	health, err := m.Health(context.Background(), "p1", "s1")
	if err != nil || health.Status != 302 || health.Responding {
		t.Fatalf("redirect=%+v %v", health, err)
	}
	modify(t, store, func(profile map[string]any) {
		profile["environments"].(map[string]any)["local"].(map[string]any)["url"] = "https://example.com"
	})
	if _, err = m.Health(context.Background(), "p1", "s1"); err == nil {
		t.Fatal("external URL accepted")
	}
}
func TestBoundedPlainTextLogs(t *testing.T) {
	b := &logBuffer{}
	_, _ = b.Write([]byte(strings.Repeat("x", 70000) + "\x1b[31mred\x1b[0m\x00"))
	text, truncated := b.snapshot()
	if !truncated || len(text) > 65536 || strings.Contains(text, "\x1b") || strings.Contains(text, "\x00") || !strings.Contains(text, "red") {
		t.Fatal("log buffer is not bounded plain text")
	}
}

func TestGoRunChildCleanup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process controls")
	}
	goTool := filepath.Join(os.Getenv("ProgramFiles"), "Go", "bin", "go.exe")
	if _, err := os.Stat(goTool); err != nil {
		t.Skip("Go tool not found")
	}
	port := availablePort(t)
	m, store, folder := testManager(t, "placeholder", &port)
	if err := os.WriteFile(filepath.Join(folder, "go.mod"), []byte("module fixture\ngo 1.27.4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := `package main
import("fmt";"net/http")
func main(){fmt.Println("GO_READY");http.HandleFunc("/health",func(w http.ResponseWriter,r *http.Request){fmt.Fprint(w,"ok")});if err:=http.ListenAndServe("127.0.0.1:PORT",nil);err!=nil{panic(err)}}`
	source = strings.ReplaceAll(source, "PORT", strconv.Itoa(port))
	if err := os.WriteFile(filepath.Join(folder, "main.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	modify(t, store, func(profile map[string]any) {
		profile["commands"].(map[string]any)["develop"] = `"` + goTool + `" run .`
	})
	plan, err := m.Prepare(context.Background(), "p1", "s1", "start")
	if err != nil {
		t.Fatal(err)
	}
	run, err := m.Start(context.Background(), plan.Token)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return strings.Contains(m.Status("p1", "s1").Logs, "GO_READY") })
	if _, err = m.Stop("p1", "s1", run.ID); err != nil {
		t.Fatal(err)
	}
	if err = portFree(&port); err != nil {
		t.Fatal("go run child survived Stop")
	}
}
