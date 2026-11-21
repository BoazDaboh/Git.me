package discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, root, path, content string) {
	t.Helper()
	path = filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestMixedProjectMetadataAndNoSecretContents(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "web/package.json", `{"name":"example-web","dependencies":{"next":"16","react":"19"},"devDependencies":{"typescript":"5"},"scripts":{"dev":"next dev --port 3015 && echo INLINE_SECRET","build":"next build","lint":"eslint","start":"next start"}}`)
	fixture(t, root, "web/package-lock.json", "{}")
	fixture(t, root, "web/.env", "DO_NOT_IMPORT=PRIVATE_VALUE")
	fixture(t, root, "api/go.mod", "module example/api\n\ngo 1.27.4\n")
	fixture(t, root, "api/cmd/service/main.go", "package main\nfunc main() { panic(\"must not run\") }")
	fixture(t, root, "api/Dockerfile", "FROM secret/DOCKER_PRIVATE_VALUE")
	fixture(t, root, ".git/HEAD", "ref: refs/heads/feat/discovery\n")
	fixture(t, root, ".git/config", "[remote \"origin\"]\n url = https://person:PRIVATE_TOKEN@github.com/example/repo.git?token=QUERY_SECRET\n")
	for _, folder := range []string{"node_modules/fake", "vendor/fake", ".next/fake", "public/fake"} {
		fixture(t, root, folder+"/package.json", `{"name":"ignored"}`)
	}
	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Services) != 2 {
		t.Fatalf("services=%+v", result.Services)
	}
	var node, goService Service
	for _, service := range result.Services {
		if service.Kind == "node" {
			node = service
		} else if service.Kind == "go" {
			goService = service
		}
	}
	if node.Commands.Install != "npm.cmd ci" || node.Commands.Develop != "npm.cmd run dev" || node.Commands.Test != "npm.cmd run lint" || node.Port == nil || *node.Port != 3015 {
		t.Fatalf("node=%+v", node)
	}
	if goService.Commands.Develop != "go run ./cmd/service" || goService.Commands.Build != "go build -o bin/service.exe ./cmd/service" || goService.Commands.Start != ".\\bin\\service.exe" || goService.Dockerfile != "Dockerfile" {
		t.Fatalf("go=%+v", goService)
	}
	if result.Git.Branch != "feat/discovery" || result.Git.Repository != "https://github.com/example/repo.git" || node.Git.Branch != result.Git.Branch {
		t.Fatalf("git=%+v", result.Git)
	}
	raw, _ := json.Marshal(result)
	for _, secret := range []string{"PRIVATE_VALUE", "PRIVATE_TOKEN", "QUERY_SECRET", "INLINE_SECRET", "DOCKER_PRIVATE_VALUE", "must not run"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("metadata leaked %s", secret)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(root, "web", ".env")); string(data) != "DO_NOT_IMPORT=PRIVATE_VALUE" {
		t.Fatal("secret file changed")
	}
}

func TestMissingScriptsLibraryAndPackageManager(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "frontend/package.json", `{"packageManager":"pnpm@10.0.0","devDependencies":{"vite":"7"},"scripts":{"build":"vite build"}}`)
	fixture(t, root, "frontend/pnpm-lock.yaml", "lockfileVersion: 9")
	fixture(t, root, "library/go.mod", "module example/library\ngo 1.27.4\n")
	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Services) != 2 {
		t.Fatalf("services=%d", len(result.Services))
	}
	for _, service := range result.Services {
		switch service.Kind {
		case "node":
			if service.PackageManager != "pnpm" || service.Commands.Build != "pnpm.cmd run build" || service.Commands.Develop != "" || service.Commands.Test != "" || service.Commands.Start != "" || service.Port != nil {
				t.Fatalf("invented script: %+v", service)
			}
		case "go":
			if service.Commands.Develop != "" || service.Commands.Start != "" || service.Commands.Build != "go build ./..." {
				t.Fatalf("invented entry point: %+v", service)
			}
		}
	}
}

func TestPHPAndStaticFiles(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "laravel/composer.json", `{"require":{"laravel/framework":"^12"}}`)
	fixture(t, root, "laravel/artisan", "<?php")
	fixture(t, root, "laravel/public/index.php", "<?php")
	fixture(t, root, "plain/public/index.php", "<?php")
	fixture(t, root, "static/index.html", "<html></html>")
	fixture(t, root, "container/Dockerfile", "FROM scratch")
	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Services) != 4 {
		t.Fatalf("services=%+v", result.Services)
	}
	for _, service := range result.Services {
		if service.RelativeFolder == "laravel" && service.Commands.Develop != "php artisan serve --host=127.0.0.1 --port=8080" {
			t.Fatal(service.Commands)
		}
		if service.RelativeFolder == "plain" && service.Commands.Develop != "php -S 127.0.0.1:8080 -t public" {
			t.Fatal(service.Commands)
		}
	}
}

func TestInvalidMetadataBoundsAndCancelledScan(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "bad/package.json", "not json")
	fixture(t, root, "large/composer.json", strings.Repeat("x", maxFileSize+1))
	fixture(t, root, "one/two/three/four/package.json", `{"name":"too-deep"}`)
	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Services) != 0 || len(result.Warnings) < 2 {
		t.Fatalf("result=%+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, root); err == nil {
		t.Fatal("cancelled scan succeeded")
	}
	for _, folder := range []string{"", "relative/folder", filepath.VolumeName(root) + string(filepath.Separator)} {
		if _, err := Scan(context.Background(), folder); err == nil {
			t.Fatalf("accepted %q", folder)
		}
	}
}

func TestLinksDoNotEscapeProject(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	fixture(t, outside, "package.json", `{"name":"outside"}`)
	if err := os.Symlink(filepath.Join(outside, "package.json"), filepath.Join(root, "package.json")); err != nil {
		t.Skip("symlink creation unavailable: " + err.Error())
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-directory")); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Services) != 0 {
		t.Fatal("scanned outside selected folder")
	}
}

func TestGitWorktreePointerAndDetachedHead(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, ".git", "gitdir: C:/outside/private/git\n")
	fixture(t, root, "index.html", "hello")
	result, err := Scan(context.Background(), root)
	if err != nil || !result.Git.Found || result.Git.Branch != "" || len(result.Warnings) == 0 {
		t.Fatalf("worktree=%+v %v", result, err)
	}
	other := t.TempDir()
	fixture(t, other, ".git/HEAD", strings.Repeat("a", 40))
	result, err = Scan(context.Background(), other)
	if err != nil || !result.Git.Detached {
		t.Fatalf("detached=%+v %v", result.Git, err)
	}
}
