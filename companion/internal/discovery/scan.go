package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxFileSize = 512 << 10
const maxDirectories = 160
const maxServices = 48

type Commands struct {
	Install string `json:"install"`
	Develop string `json:"develop"`
	Build   string `json:"build"`
	Test    string `json:"test"`
	Start   string `json:"start"`
}
type Git struct {
	Found      bool   `json:"found"`
	Branch     string `json:"branch"`
	Repository string `json:"repository"`
	Detached   bool   `json:"detached"`
	Commit     string `json:"commit"`
}
type Service struct {
	Key            string   `json:"key"`
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	Folder         string   `json:"folder"`
	RelativeFolder string   `json:"relative_folder"`
	Role           string   `json:"role"`
	Runtime        string   `json:"runtime"`
	Stack          []string `json:"stack"`
	Commands       Commands `json:"commands"`
	Port           *int     `json:"port"`
	Files          []string `json:"files"`
	Notes          []string `json:"notes"`
	Git            Git      `json:"git"`
	Dockerfile     string   `json:"dockerfile"`
	PackageManager string   `json:"package_manager,omitempty"`
	Checks         []Check  `json:"checks"`
}

type Check struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Slot    string `json:"slot"`
}
type Result struct {
	Folder      string    `json:"folder"`
	ScannedAt   string    `json:"scanned_at"`
	Directories int       `json:"directories"`
	Services    []Service `json:"services"`
	Git         Git       `json:"git"`
	Warnings    []string  `json:"warnings"`
}
type scanner struct {
	root   *os.Root
	ctx    context.Context
	result Result
	reads  int
}

var skip = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "build": true, "out": true, "coverage": true, "bin": true, "obj": true, "storage": true, "target": true, "venv": true, "__pycache__": true, "data": true, "backups": true, "public": true}
var safeScript = regexp.MustCompile(`^[A-Za-z0-9:_-]{1,64}$`)
var portFlag = regexp.MustCompile(`(?:^|\s)(?:--port|-p)(?:=|\s+)([0-9]{1,5})(?:\s|$)`)
var engine = regexp.MustCompile(`^[0-9xXv.^~<>=| +*-]{1,64}$`)

func Scan(ctx context.Context, folder string) (Result, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" || !filepath.IsAbs(folder) || strings.HasPrefix(folder, `\\`) || strings.HasPrefix(folder, "//") {
		return Result{}, errors.New("choose an absolute local project folder")
	}
	folder = filepath.Clean(folder)
	if filepath.Dir(folder) == folder {
		return Result{}, errors.New("choose a project folder, not a drive root")
	}
	resolved, err := filepath.EvalSymlinks(folder)
	if err != nil {
		return Result{}, errors.New("project folder was not found or cannot be opened")
	}
	if filepath.Dir(resolved) == resolved || strings.HasPrefix(resolved, `\\`) {
		return Result{}, errors.New("choose a local project folder")
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return Result{}, errors.New("project folder cannot be opened")
	}
	defer root.Close()
	s := &scanner{root: root, ctx: ctx, result: Result{Folder: folder, ScannedAt: time.Now().UTC().Format(time.RFC3339), Services: []Service{}, Warnings: []string{}}}
	s.result.Git = s.git(".")
	if err := s.walk(".", 0, s.result.Git); err != nil {
		return Result{}, err
	}
	return s.result, nil
}

func (s *scanner) warn(message string) {
	for _, old := range s.result.Warnings {
		if old == message {
			return
		}
	}
	if len(s.result.Warnings) < 30 {
		s.result.Warnings = append(s.result.Warnings, message)
	}
}

func (s *scanner) read(path string) ([]byte, bool) {
	if s.ctx.Err() != nil {
		return nil, false
	}
	if s.reads >= 240 {
		s.warn("File-read limit reached. Scan a smaller project folder for the remaining services.")
		return nil, false
	}
	info, err := s.root.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileSize {
		s.warn(filepath.ToSlash(path) + ": skipped unreadable, linked or oversized metadata.")
		return nil, false
	}
	file, err := s.root.Open(path)
	if err != nil {
		s.warn(filepath.ToSlash(path) + ": metadata could not be opened.")
		return nil, false
	}
	defer file.Close()
	s.reads++
	data, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil || len(data) > maxFileSize {
		s.warn(filepath.ToSlash(path) + ": metadata read limit reached.")
		return nil, false
	}
	return data, true
}

func (s *scanner) exists(path string) bool {
	info, err := s.root.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func (s *scanner) directories(path string, limit int) ([]os.DirEntry, error) {
	f, err := s.root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(limit + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > limit {
		entries = entries[:limit]
		s.warn("Directory entry limit reached. Scan a smaller folder if a service is missing.")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (s *scanner) walk(relative string, depth int, inherited Git) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if s.result.Directories >= maxDirectories || len(s.result.Services) >= maxServices {
		s.warn("Scan limit reached. Select a smaller folder to find the remaining services.")
		return nil
	}
	s.result.Directories++
	git := inherited
	if relative != "." {
		if found := s.git(relative); found.Found {
			git = found
		}
	}
	before := len(s.result.Services)
	if s.exists(filepath.Join(relative, "package.json")) {
		s.node(relative, git)
	}
	if s.exists(filepath.Join(relative, "go.mod")) {
		s.golang(relative, git)
	}
	if s.exists(filepath.Join(relative, "composer.json")) {
		s.php(relative, git, true)
	} else if s.exists(filepath.Join(relative, "index.php")) || s.exists(filepath.Join(relative, "public", "index.php")) {
		s.php(relative, git, false)
	}
	if len(s.result.Services) == before && s.exists(filepath.Join(relative, "index.html")) {
		service := s.base(relative, "browser", "frontend", "Browser", []string{"HTML", "CSS"}, git)
		service.Files = []string{"index.html"}
		service.Notes = append(service.Notes, "Static entry file found. Choose a development server manually if needed.")
		s.add(service)
	}
	if len(s.result.Services) == before && s.exists(filepath.Join(relative, "Dockerfile")) {
		service := s.base(relative, "container", "other", "Docker", []string{"Docker"}, git)
		service.Files = []string{"Dockerfile"}
		service.Notes = append(service.Notes, "Container file found. Runtime and commands need your review.")
		s.add(service)
	}
	if depth == 3 {
		return nil
	}
	entries, err := s.directories(relative, 256)
	if err != nil {
		if depth == 0 {
			return errors.New("project directory cannot be listed")
		}
		s.warn(filepath.ToSlash(relative) + ": subfolders could not be listed.")
		return nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(name, ".") || skip[strings.ToLower(name)] {
			continue
		}
		if err := s.walk(filepath.Join(relative, name), depth+1, git); err != nil {
			return err
		}
	}
	return nil
}

func (s *scanner) base(relative, kind, role, runtime string, stack []string, git Git) Service {
	name := filepath.Base(s.result.Folder)
	if relative != "." {
		name = filepath.Base(relative)
	}
	service := Service{Key: filepath.ToSlash(relative) + ":" + kind, Kind: kind, Name: name, Folder: filepath.Join(s.result.Folder, relative), RelativeFolder: filepath.ToSlash(relative), Role: role, Runtime: runtime, Stack: stack, Files: []string{}, Notes: []string{}, Git: git}
	if s.exists(filepath.Join(relative, "Dockerfile")) {
		service.Dockerfile = "Dockerfile"
		service.Stack = append(service.Stack, "Docker")
	}
	return service
}
func (s *scanner) add(service Service) {
	if len(s.result.Services) < maxServices {
		s.result.Services = append(s.result.Services, service)
	} else {
		s.warn("Service limit reached. Scan a smaller folder.")
	}
}

func (s *scanner) node(relative string, git Git) {
	data, ok := s.read(filepath.Join(relative, "package.json"))
	if !ok {
		return
	}
	var manifest struct {
		Name            string                     `json:"name"`
		PackageManager  string                     `json:"packageManager"`
		Scripts         map[string]string          `json:"scripts"`
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
		Engines         struct {
			Node string `json:"node"`
		} `json:"engines"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		s.warn(filepath.ToSlash(filepath.Join(relative, "package.json")) + ": invalid metadata; skipped.")
		return
	}
	stack := []string{"Node.js"}
	has := func(name string) bool {
		_, a := manifest.Dependencies[name]
		_, b := manifest.DevDependencies[name]
		return a || b
	}
	for _, item := range [][2]string{{"next", "Next.js"}, {"react", "React"}, {"@nestjs/core", "NestJS"}, {"express", "Express"}, {"vite", "Vite"}, {"vue", "Vue"}, {"nuxt", "Nuxt"}, {"svelte", "Svelte"}, {"astro", "Astro"}, {"typescript", "TypeScript"}, {"tailwindcss", "Tailwind CSS"}} {
		if has(item[0]) {
			stack = append(stack, item[1])
		}
	}
	if !has("typescript") {
		stack = append(stack, "JavaScript")
	}
	role := "backend"
	if has("next") || has("react") || has("vite") || has("vue") || has("nuxt") || has("svelte") || has("astro") {
		role = "frontend"
	}
	service := s.base(relative, "node", role, "Node.js", stack, git)
	service.Files = []string{"package.json"}
	if engine.MatchString(manifest.Engines.Node) {
		service.Runtime += " " + manifest.Engines.Node
	}
	if len(manifest.Name) > 0 && len(manifest.Name) <= 80 && !strings.ContainsAny(manifest.Name, "\r\n") {
		service.Name = manifest.Name
	}
	manager := "npm"
	declared := strings.Split(manifest.PackageManager, "@")[0]
	for _, candidate := range []struct{ name, file string }{{"pnpm", "pnpm-lock.yaml"}, {"yarn", "yarn.lock"}, {"bun", "bun.lock"}, {"bun", "bun.lockb"}, {"npm", "package-lock.json"}} {
		if s.exists(filepath.Join(relative, candidate.file)) {
			manager = candidate.name
			service.Files = append(service.Files, candidate.file)
			break
		}
	}
	if declared != "" {
		if declared == "npm" || declared == "pnpm" || declared == "yarn" || declared == "bun" {
			manager = declared
		} else {
			service.Notes = append(service.Notes, "Unrecognized package manager. Review the npm suggestions.")
		}
	}
	service.PackageManager = manager
	runner := manager + ".cmd"
	if manager == "bun" {
		runner = "bun"
	}
	service.Commands.Install = runner + " install"
	if manager == "npm" && s.exists(filepath.Join(relative, "package-lock.json")) {
		service.Commands.Install = "npm.cmd ci"
	}
	command := func(names ...string) (string, string) {
		for _, name := range names {
			if script, exists := manifest.Scripts[name]; exists && script != "" && safeScript.MatchString(name) {
				return runner + " run " + name, script
			}
		}
		return "", ""
	}
	var devScript string
	service.Commands.Develop, devScript = command("dev", "develop", "serve")
	service.Commands.Build, _ = command("build")
	service.Commands.Test, _ = command("test", "test:unit", "lint")
	service.Commands.Start, _ = command("start")
	service.Checks = []Check{}
	for _, check := range []struct {
		name, slot string
		scripts    []string
	}{{"Lint", "lint", []string{"lint"}}, {"Type check", "types", []string{"typecheck", "type-check", "check:types"}}, {"Tests", "test", []string{"test", "test:unit"}}, {"Build", "build", []string{"build"}}} {
		cmd, body := command(check.scripts...)
		if cmd != "" && !strings.Contains(body, "Error: no test specified") {
			service.Checks = append(service.Checks, Check{check.name, cmd, check.slot})
		}
	}
	if match := portFlag.FindStringSubmatch(devScript); len(match) > 1 {
		value, _ := strconv.Atoi(match[1])
		if value > 0 && value <= 65535 {
			service.Port = &value
		}
	}
	if service.Port == nil && service.Commands.Develop != "" {
		value := 0
		if has("next") {
			value = 3000
		} else if has("vite") {
			value = 5173
		}
		if value > 0 {
			service.Port = &value
			service.Notes = append(service.Notes, "Port is a framework default; confirm it in Local setup.")
		}
	}
	service.Notes = append(service.Notes, "Commands reference declared scripts. Script bodies are not imported or executed.")
	s.add(service)
}

func (s *scanner) golang(relative string, git Git) {
	data, ok := s.read(filepath.Join(relative, "go.mod"))
	if !ok {
		return
	}
	version, module := "", ""
	lines := bufio.NewScanner(strings.NewReader(string(data)))
	for lines.Scan() {
		fields := strings.Fields(strings.SplitN(lines.Text(), "//", 2)[0])
		if len(fields) >= 2 {
			if fields[0] == "go" && regexp.MustCompile(`^[0-9.]{1,20}$`).MatchString(fields[1]) {
				version = fields[1]
			}
			if fields[0] == "module" {
				module = strings.Trim(fields[1], `"`)
			}
		}
	}
	if module == "" {
		s.warn(filepath.ToSlash(filepath.Join(relative, "go.mod")) + ": missing module directive; skipped.")
		return
	}
	entries := []string{}
	if s.exists(filepath.Join(relative, "main.go")) {
		entries = append(entries, ".")
	}
	if info, err := s.root.Lstat(filepath.Join(relative, "cmd")); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		if children, err := s.directories(filepath.Join(relative, "cmd"), 32); err == nil {
			for _, child := range children {
				if child.IsDir() && child.Type()&os.ModeSymlink == 0 && safeScript.MatchString(child.Name()) && s.exists(filepath.Join(relative, "cmd", child.Name(), "main.go")) {
					entries = append(entries, "./cmd/"+child.Name())
				}
			}
		}
	}
	if len(entries) == 0 {
		entries = append(entries, "")
	}
	if len(entries) > 8 {
		entries = entries[:8]
		s.warn("Go entry-point limit reached. Review additional cmd folders manually.")
	}
	for _, entry := range entries {
		service := s.base(relative, "go", "backend", "Go", []string{"Go"}, git)
		if version != "" {
			service.Runtime += " " + version
		}
		service.Files = []string{"go.mod"}
		service.Commands = Commands{Install: "go mod download", Build: "go build ./...", Test: "go test ./..."}
		service.Checks = []Check{{"Tests", "go test ./...", "test"}, {"Vet", "go vet ./...", "vet"}, {"Build", "go build ./...", "build"}}
		if entry != "" {
			name := "app"
			if entry != "." {
				name = filepath.Base(entry)
			}
			service.Key += ":" + entry
			service.Name = filepath.Base(service.Folder) + " " + name
			service.Commands.Develop = "go run " + entry
			service.Commands.Build = "go build -o bin/" + name + ".exe " + entry
			service.Commands.Start = ".\\bin\\" + name + ".exe"
			service.Files = append(service.Files, filepath.ToSlash(filepath.Join(strings.TrimPrefix(entry, "./"), "main.go")))
			service.Notes = append(service.Notes, "Entry point inferred from main.go. Build and runtime behavior are not tested.")
		} else {
			service.Notes = append(service.Notes, "No root or cmd/*/main.go entry point found. Add run commands manually for a library or another layout.")
		}
		s.add(service)
	}
}

func (s *scanner) php(relative string, git Git, composer bool) {
	service := s.base(relative, "php", "backend", "PHP", []string{"PHP"}, git)
	service.Checks = []Check{}
	for _, entry := range []string{"index.php", "public/index.php"} {
		if s.exists(filepath.Join(relative, filepath.FromSlash(entry))) {
			service.Checks = append(service.Checks, Check{"PHP syntax: " + entry, "php -l " + entry, "syntax"})
		}
	}
	public := s.exists(filepath.Join(relative, "public", "index.php"))
	service.Commands.Develop = "php -S 127.0.0.1:8080"
	if public {
		service.Commands.Develop += " -t public"
	}
	port := 8080
	service.Port = &port
	if composer {
		data, ok := s.read(filepath.Join(relative, "composer.json"))
		if !ok {
			return
		}
		var manifest struct {
			Require    map[string]json.RawMessage `json:"require"`
			RequireDev map[string]json.RawMessage `json:"require-dev"`
			Scripts    map[string]json.RawMessage `json:"scripts"`
		}
		if json.Unmarshal(data, &manifest) != nil {
			s.warn(filepath.ToSlash(filepath.Join(relative, "composer.json")) + ": invalid metadata; skipped.")
			return
		}
		service.Stack = append(service.Stack, "Composer")
		service.Files = append(service.Files, "composer.json")
		service.Commands.Install = "composer install"
		service.Checks = append(service.Checks, Check{"Composer validation", "composer validate", "validate"})
		if _, laravel := manifest.Require["laravel/framework"]; laravel && s.exists(filepath.Join(relative, "artisan")) {
			service.Stack = append(service.Stack, "Laravel")
			service.Files = append(service.Files, "artisan")
			service.Commands.Develop = "php artisan serve --host=127.0.0.1 --port=8080"
			service.Commands.Test = "php artisan test"
		} else {
			if _, exists := manifest.Scripts["test"]; exists {
				service.Commands.Test = "composer run-script test"
			}
			if _, exists := manifest.Scripts["dev"]; exists {
				service.Commands.Develop = "composer run-script dev"
				service.Port = nil
			}
		}
		service.Notes = append(service.Notes, "Composer and PHP commands are suggestions. No dependencies are installed.")
		for _, check := range []struct{ name, script, slot string }{{"Lint", "lint", "lint"}, {"Type check", "typecheck", "types"}, {"Tests", "test", "test"}, {"Build", "build", "build"}} {
			if raw, exists := manifest.Scripts[check.script]; exists && validComposerScript(raw) {
				service.Checks = append(service.Checks, Check{check.name, "composer run-script " + check.script, check.slot})
			}
		}
		if service.Commands.Test == "php artisan test" {
			found := false
			for _, check := range service.Checks {
				if check.Slot == "test" {
					found = true
				}
			}
			if !found {
				service.Checks = append(service.Checks, Check{"Laravel tests", "php artisan test", "test"})
			}
		}
	} else {
		if s.exists(filepath.Join(relative, "index.php")) {
			service.Files = append(service.Files, "index.php")
		} else {
			service.Files = append(service.Files, "public/index.php")
		}
	}
	if service.Port != nil {
		service.Notes = append(service.Notes, "Port 8080 is suggested for local PHP development; change it if occupied.")
	}
	s.add(service)
}

func validComposerScript(raw json.RawMessage) bool {
	var command string
	if json.Unmarshal(raw, &command) == nil {
		return strings.TrimSpace(command) != ""
	}
	var commands []string
	if json.Unmarshal(raw, &commands) != nil || len(commands) == 0 {
		return false
	}
	for _, item := range commands {
		if strings.TrimSpace(item) == "" {
			return false
		}
	}
	return true
}

func (s *scanner) git(relative string) Git {
	path := filepath.Join(relative, ".git")
	info, err := s.root.Lstat(path)
	if err != nil {
		return Git{}
	}
	result := Git{Found: true}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		s.warn(filepath.ToSlash(path) + ": linked Git metadata was not read outside the project folder.")
		return result
	}
	if data, ok := s.read(filepath.Join(path, "HEAD")); ok {
		head := strings.TrimSpace(string(data))
		if strings.HasPrefix(head, "ref: refs/heads/") {
			branch := strings.TrimPrefix(head, "ref: refs/heads/")
			if len(branch) <= 100 && !strings.ContainsAny(branch, "\r\n") {
				result.Branch = branch
				if safeRef(branch) {
					ref := filepath.Join(path, "refs", "heads", filepath.FromSlash(branch))
					if s.plainParents(ref) {
						if value, ok := s.read(ref); ok && commitPattern.MatchString(strings.TrimSpace(string(value))) {
							result.Commit = strings.TrimSpace(string(value))
						}
					}
					if result.Commit == "" {
						if packed, ok := s.read(filepath.Join(path, "packed-refs")); ok {
							for _, line := range strings.Split(string(packed), "\n") {
								parts := strings.Fields(line)
								if len(parts) == 2 && parts[1] == "refs/heads/"+branch && commitPattern.MatchString(parts[0]) {
									result.Commit = parts[0]
									break
								}
							}
						}
					}
				}
			}
		} else if regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`).MatchString(head) {
			result.Detached = true
			if commitPattern.MatchString(head) {
				result.Commit = head
			}
		}
	}
	if data, ok := s.read(filepath.Join(path, "config")); ok {
		section := ""
		lines := bufio.NewScanner(strings.NewReader(string(data)))
		for lines.Scan() {
			line := strings.TrimSpace(lines.Text())
			if strings.HasPrefix(line, "[") {
				section = line
				continue
			}
			if section != `[remote "origin"]` {
				continue
			}
			key, value, found := strings.Cut(line, "=")
			if found && strings.TrimSpace(key) == "url" {
				result.Repository = cleanRepository(strings.Trim(strings.TrimSpace(value), `"`))
				break
			}
		}
	}
	return result
}

func cleanRepository(raw string) string {
	if len(raw) > 500 || strings.ContainsAny(raw, "\r\n\t ") {
		return ""
	}
	if regexp.MustCompile(`^git@[A-Za-z0-9.-]+:[A-Za-z0-9_./-]+$`).MatchString(raw) {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme == "http" || u.Scheme == "https" {
		u.User = nil
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}
	if u.Scheme == "ssh" && u.User != nil && u.User.Username() == "git" {
		u.User = url.User("git")
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}
	return ""
}
