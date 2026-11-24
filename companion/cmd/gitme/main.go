package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitme/companion/internal/credentials"
	"gitme/companion/internal/discovery"
	"gitme/companion/internal/github"
	"gitme/companion/internal/localgit"
	"gitme/companion/internal/processes"
	"gitme/companion/internal/vercel"
	"gitme/companion/internal/workspace"
)

const address = "127.0.0.1:47821"

type healthResponse struct {
	App          string   `json:"app"`
	Status       string   `json:"status"`
	Protocol     int      `json:"protocol"`
	Version      string   `json:"version"`
	StartedAt    string   `json:"started_at"`
	LastSleep    int64    `json:"last_sleep_ms,omitempty"`
	Capabilities []string `json:"capabilities"`
}

func newHandler(startedAt time.Time, store *workspace.Store) http.Handler {
	return newHandlerWithManager(startedAt, store, processes.New(store))
}

func newHandlerWithManager(startedAt time.Time, store *workspace.Store, manager *processes.Manager) http.Handler {
	accounts := github.NewConnections(github.New(credentials.New(), nil), &credentials.Store{Target: "GitMe/GitHub/connections"}, func(id string) credentials.Vault { return &credentials.Store{Target: "GitMe/GitHub/accounts/" + id} })
	vercelService := vercel.New(&credentials.Store{Target: "GitMe/Vercel/connections"}, func(id string) credentials.Vault { return &credentials.Store{Target: "GitMe/Vercel/accounts/" + id} }, nil)
	return newHandlerWithCloud(startedAt, store, manager, accounts, vercelService)
}

func newHandlerWithCloud(startedAt time.Time, store *workspace.Store, manager *processes.Manager, githubService *github.Connections, vercelService *vercel.Service) http.Handler {
	mux := http.NewServeMux()
	registerProjectTemplates(mux)
	registerAPIRequests(mux, store)
	registerProcesses(mux, manager)
	registerGitHub(mux, store, githubService)
	registerVercel(mux, store, vercelService)
	registerReleases(mux, store)
	gitManager := localgit.New()
	registerLocalGit(mux, store, gitManager)
	registerDeploymentTracking(mux, store, gitManager, vercelService)
	registerDeploymentActions(mux, store, vercelService)
	scanning := make(chan struct{}, 1)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(healthResponse{
			App: "gitme-companion", Status: "ok", Protocol: 1, Version: "0.17.0",
			StartedAt: startedAt.UTC().Format(time.RFC3339), LastSleep: lastSleepMillis(), Capabilities: []string{"health", "workspace", "project-scan", "processes", "github", "github-accounts", "vercel", "release-inspection", "local-git", "git-push", "deployment-tracking", "reviewed-deployments", "local-uploads", "api-testing", "reviewed-api-posts", "api-status-checks"},
		}); err != nil {
			log.Printf("Health response failed: %v", err)
		}
	})
	mux.HandleFunc("POST /projects/scan", func(w http.ResponseWriter, r *http.Request) {
		contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if contentType != "application/json" || r.Header.Get("X-GitMe-Client") != "workspace-v1" {
			http.Error(w, "JSON and X-GitMe-Client: workspace-v1 required", 415)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var input struct {
			ProjectID string `json:"project_id"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.ProjectID == "" || len(input.ProjectID) > 200 {
			http.Error(w, "Choose a saved project", 400)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "Expected one JSON object", 400)
			return
		}
		select {
		case scanning <- struct{}{}:
			defer func() { <-scanning }()
		default:
			http.Error(w, "Another project scan is running. Retry shortly.", 409)
			return
		}
		snapshot, err := store.Load(r.Context())
		if err != nil {
			http.Error(w, "Could not read saved projects", 500)
			return
		}
		var data struct {
			Projects []struct {
				ID     string `json:"id"`
				Folder string `json:"folder"`
			} `json:"projects"`
		}
		if json.Unmarshal(snapshot.Workspace, &data) != nil {
			http.Error(w, "Save the workspace before scanning", 409)
			return
		}
		folder, found := "", false
		for _, project := range data.Projects {
			if project.ID == input.ProjectID {
				folder, found = project.Folder, true
				break
			}
		}
		if !found {
			http.Error(w, "Saved project not found", 404)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		result, err := discovery.Scan(ctx, folder)
		if err != nil {
			if ctx.Err() != nil {
				http.Error(w, "Scan timed out. Choose a smaller project folder.", 408)
			} else {
				http.Error(w, err.Error(), 422)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("OPTIONS /projects/scan", func(w http.ResponseWriter, r *http.Request) {
		method := r.Header.Get("Access-Control-Request-Method")
		if method != "" && method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitMe-Client")
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /workspace", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := store.Load(r.Context())
		if err != nil {
			log.Printf("Read workspace: %v", err)
			http.Error(w, "Database read failed", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			workspace.Snapshot
			Path string `json:"database_path"`
		}{snapshot, store.Path})
	})
	mux.HandleFunc("PUT /workspace", func(w http.ResponseWriter, r *http.Request) {
		contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if contentType != "application/json" || r.Header.Get("X-GitMe-Client") != "workspace-v1" {
			http.Error(w, "JSON and X-GitMe-Client: workspace-v1 required", 415)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
		var input struct {
			DatabaseID string          `json:"database_id"`
			Revision   int64           `json:"revision"`
			Workspace  json.RawMessage `json:"workspace"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(w, "Workspace exceeds 20 MB", 413)
			} else {
				http.Error(w, "Invalid workspace request", 400)
			}
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "Expected one JSON object", 400)
			return
		}
		if input.DatabaseID == "" || input.Revision < 0 {
			http.Error(w, "Database identity and revision required", 400)
			return
		}
		if _, err := workspace.Validate(input.Workspace); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		revision, err := store.Save(r.Context(), input.DatabaseID, input.Revision, input.Workspace)
		if errors.Is(err, workspace.ErrConflict) {
			http.Error(w, "Workspace changed. Export pending edits and load the database copy.", 409)
			return
		}
		if err != nil {
			log.Printf("Save workspace: %v", err)
			http.Error(w, "Database save failed", 500)
			return
		}
		if err := ensureTimerCheckpoint(r.Context(), store, input.DatabaseID, input.Workspace); err != nil {
			log.Printf("Timer checkpoint: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"database_id": input.DatabaseID, "revision": revision})
	})
	mux.HandleFunc("OPTIONS /workspace", func(w http.ResponseWriter, r *http.Request) {
		method := r.Header.Get("Access-Control-Request-Method")
		if method != "GET" && method != "PUT" && method != "" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitMe-Client")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("OPTIONS /health", func(w http.ResponseWriter, r *http.Request) {
		method := r.Header.Get("Access-Control-Request-Method")
		if method != "" && method != http.MethodGet && method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
	})

	guarded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Vary", "Origin")
		if r.Host != address && r.Host != "localhost:47821" {
			http.Error(w, "Host not allowed", http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			if origin != "http://127.0.0.1:3015" && origin != "http://localhost:3015" {
				http.Error(w, "Origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		mux.ServeHTTP(w, r)
	})
	return &gitHandler{Handler: guarded, git: gitManager}
}

type gitHandler struct {
	http.Handler
	git *localgit.Manager
}

func (h *gitHandler) Close() error { h.git.Close(); return nil }

func main() {
	stopPower := watchSystemSleep()
	defer stopPower()
	path, err := workspace.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}
	store, err := workspace.Open(path)
	if err != nil {
		log.Fatalf("Open database: %v", err)
	}
	defer store.Close()
	manager := processes.New(store)
	defer manager.Close()
	server := &http.Server{
		Addr: address, Handler: newHandlerWithManager(time.Now(), store, manager),
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 110 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8 << 10,
	}
	defer server.Handler.(io.Closer).Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Start companion: %v", err)
	}
	defer listener.Close()
	if _, err := recoverTimerCheckpoint(ctx, store, time.Now().UnixMilli()); err != nil {
		log.Printf("Timer recovery: %v", err)
	}
	timerDone := make(chan struct{})
	go func() { defer close(timerDone); runTimerCheckpoints(ctx, store) }()
	defer func() { stop(); <-timerDone }()
	go func() {
		<-ctx.Done()
		<-timerDone
		manager.Close()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("Shutdown: %v", err)
		}
	}()
	log.Printf("Git/Me companion: http://%s/health", address)
	log.Printf("SQLite database: %s", store.Path)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
