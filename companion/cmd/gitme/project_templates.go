package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type templateInput struct {
	Folder   string `json:"folder"`
	Frontend string `json:"frontend"`
	Backend  string `json:"backend"`
}

func createProjectTemplate(input templateInput) error {
	folder := filepath.Clean(strings.TrimSpace(input.Folder))
	if !filepath.IsAbs(folder) || folder == filepath.VolumeName(folder)+string(os.PathSeparator) || folder == "." {
		return errors.New("choose an absolute project folder")
	}
	if input.Frontend != "next" && input.Frontend != "react" && input.Frontend != "static" {
		return errors.New("unsupported frontend template")
	}
	if input.Backend != "go" && input.Backend != "node" && input.Backend != "php" {
		return errors.New("unsupported backend template")
	}
	parent, name := filepath.Dir(folder), filepath.Base(folder)
	if name == "." || name == ".." || strings.ContainsAny(name, `<>:"|?*`) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return errors.New("invalid folder name")
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return errors.New("project parent folder must already exist and be accessible")
	}
	defer root.Close()
	if err = root.Mkdir(name, 0755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("destination already exists; choose another folder")
		}
		return errors.New("could not create project folder")
	}
	dest, err := root.OpenRoot(name)
	if err != nil {
		return errors.New("folder was created but could not be opened; inspect it before retrying")
	}
	defer dest.Close()
	dirs := []string{"private", "web", "api"}
	if input.Frontend == "static" {
		dirs = append(dirs, "web/css", "web/js")
	} else {
		dirs = append(dirs, "web/public", "web/src", "web/src/components")
		if input.Frontend == "next" {
			dirs = append(dirs, "web/src/app")
		}
	}
	if input.Backend == "go" {
		dirs = append(dirs, "api/cmd", "api/cmd/server", "api/internal", "api/migrations")
	} else if input.Backend == "node" {
		dirs = append(dirs, "api/src", "api/tests")
	} else {
		dirs = append(dirs, "api/public", "api/src", "api/tests")
	}
	for _, dir := range dirs {
		if err = dest.Mkdir(dir, 0755); err != nil {
			return fmt.Errorf("folder partially created; inspect %s before retrying", folder)
		}
	}
	files := map[string]string{"README.md": "# Project\n\nFrontend: web/\nBackend API: api/\n\nFolder structure only. Set up your chosen frameworks separately.\n", ".gitignore": "node_modules/\n.next/\ndist/\nbuild/\n.env\n.env.*\n!.env.example\n/private/\n", "private/.gitignore": "*\n!.gitignore\n", "web/README.md": "# Frontend\n\nSet up the selected frontend framework here.\n", "api/README.md": "# API\n\nSet up the selected backend here.\n"}
	if input.Frontend == "static" {
		files["web/index.html"] = "<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>Project</title></head><body></body></html>\n"
	}
	for _, dir := range dirs {
		if dir != "private" {
			files[dir+"/.gitkeep"] = ""
		}
	}
	for path, contents := range files {
		f, e := dest.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e != nil {
			return fmt.Errorf("folder partially created; inspect %s before retrying", folder)
		}
		_, e = f.WriteString(contents)
		closeErr := f.Close()
		if e != nil || closeErr != nil {
			return fmt.Errorf("folder partially created; inspect %s before retrying", folder)
		}
	}
	return nil
}
func registerProjectTemplates(mux *http.ServeMux) {
	mux.HandleFunc("POST /projects/create-template", func(w http.ResponseWriter, r *http.Request) {
		var input templateInput
		if !processJSON(w, r, &input) {
			return
		}
		if err := createProjectTemplate(input); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		respondJSON(w, 201, map[string]bool{"created": true})
	})
	mux.HandleFunc("OPTIONS /projects/create-template", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitMe-Client")
		w.WriteHeader(204)
	})
}
