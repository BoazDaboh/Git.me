package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateProjectTemplate(t *testing.T) {
	for _, front := range []string{"next", "react", "static"} {
		for _, back := range []string{"go", "node", "php"} {
			t.Run(front+"-"+back, func(t *testing.T) {
				root := t.TempDir()
				target := filepath.Join(root, "sample")
				input := templateInput{Folder: target, Frontend: front, Backend: back}
				if err := createProjectTemplate(input); err != nil {
					t.Fatal(err)
				}
				for _, path := range []string{"README.md", ".gitignore", "private/.gitignore", "web/README.md", "api/README.md"} {
					if _, err := os.Stat(filepath.Join(target, path)); err != nil {
						t.Fatal(err)
					}
				}
				marker := filepath.Join(target, "keep.txt")
				if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := createProjectTemplate(input); err == nil {
					t.Fatal("existing folder accepted")
				}
				data, err := os.ReadFile(marker)
				if err != nil || string(data) != "keep" {
					t.Fatal("existing data changed")
				}
			})
		}
	}
}
func TestCreateProjectTemplateRejectsInvalid(t *testing.T) {
	parent := t.TempDir()
	input := templateInput{Folder: filepath.Join(parent, "sample"), Frontend: "unknown", Backend: "go"}
	if createProjectTemplate(input) == nil {
		t.Fatal("invalid stack accepted")
	}
	if _, err := os.Stat(input.Folder); !os.IsNotExist(err) {
		t.Fatal("invalid request created folder")
	}
}
