package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gitme/companion/internal/workspace"
)

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Draft struct {
	ID       string `json:"id"`
	Project  string `json:"project"`
	Platform string `json:"platform"`
	Text     string `json:"text"`
}
type CommitContext struct {
	SHA   string `json:"sha"`
	Title string `json:"title"`
	Date  string `json:"date"`
}
type PostContext struct {
	ProjectID   string          `json:"projectId"`
	ProjectName string          `json:"projectName"`
	Description string          `json:"description"`
	Repository  string          `json:"repository"`
	Commits     []CommitContext `json:"commits"`
	Extra       string          `json:"extra"`
	Platform    string          `json:"platform"`
	Tone        string          `json:"tone"`
	SavedAt     int64           `json:"savedAt"`
}
type State struct {
	Projects []Project     `json:"projects"`
	Contexts []PostContext `json:"postContexts"`
	Drafts   []Draft       `json:"drafts"`
}
type ProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"ID of the project whose drafts to read"`
}
type DraftInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID"`
	Platform  string `json:"platform" jsonschema:"LinkedIn, X, or Other"`
	Text      string `json:"text" jsonschema:"Draft text, no more than 5000 characters"`
}
type UpdateInput struct {
	ID           string `json:"id" jsonschema:"Draft ID"`
	ExpectedText string `json:"expected_text" jsonschema:"Exact currently saved draft text, used to prevent overwriting changes"`
	Text         string `json:"text" jsonschema:"Replacement draft text"`
}
type Output struct {
	Message  string `json:"message"`
	Revision int64  `json:"revision"`
}

func read(ctx context.Context, store *workspace.Store) (workspace.Snapshot, State, error) {
	snap, err := store.Load(ctx)
	if err != nil {
		return snap, State{}, err
	}
	if snap.Revision == 0 {
		return snap, State{}, errors.New("save the workspace in Git/Me first")
	}
	var state State
	if err = json.Unmarshal(snap.Workspace, &state); err != nil {
		return snap, state, err
	}
	return snap, state, nil
}
func saveDraft(ctx context.Context, store *workspace.Store, draft Draft, expected *string) (Output, error) {
	if len([]rune(draft.Text)) > 5000 || strings.TrimSpace(draft.Text) == "" {
		return Output{}, errors.New("draft text must contain 1 to 5000 characters")
	}
	snap, state, err := read(ctx, store)
	if err != nil {
		return Output{}, err
	}
	if expected == nil {
		found := false
		for _, p := range state.Projects {
			if p.ID == draft.Project {
				found = true
			}
		}
		if !found {
			return Output{}, errors.New("project not found")
		}
		if draft.Platform != "LinkedIn" && draft.Platform != "X" && draft.Platform != "Other" {
			return Output{}, errors.New("platform must be LinkedIn, X, or Other")
		}
		b := make([]byte, 16)
		if _, err = rand.Read(b); err != nil {
			return Output{}, err
		}
		draft.ID = hex.EncodeToString(b)
		state.Drafts = append([]Draft{draft}, state.Drafts...)
	} else {
		found := false
		for i, d := range state.Drafts {
			if d.ID == draft.ID {
				found = true
				if d.Text != *expected {
					return Output{}, errors.New("draft changed; read it again before updating")
				}
				state.Drafts[i].Text = draft.Text
				break
			}
		}
		if !found {
			return Output{}, errors.New("draft not found")
		}
	}
	var full map[string]json.RawMessage
	if err = json.Unmarshal(snap.Workspace, &full); err != nil {
		return Output{}, err
	}
	var originalDrafts []json.RawMessage
	if err = json.Unmarshal(full["drafts"], &originalDrafts); err != nil {
		return Output{}, err
	}
	if expected == nil {
		added, marshalErr := json.Marshal(draft)
		if marshalErr != nil {
			return Output{}, marshalErr
		}
		originalDrafts = append([]json.RawMessage{added}, originalDrafts...)
	} else {
		for i, rawDraft := range originalDrafts {
			var fields map[string]json.RawMessage
			if json.Unmarshal(rawDraft, &fields) != nil {
				return Output{}, errors.New("invalid draft")
			}
			var id string
			_ = json.Unmarshal(fields["id"], &id)
			if id == draft.ID {
				fields["text"], _ = json.Marshal(draft.Text)
				originalDrafts[i], err = json.Marshal(fields)
				if err != nil {
					return Output{}, err
				}
				break
			}
		}
	}
	full["drafts"], err = json.Marshal(originalDrafts)
	if err != nil {
		return Output{}, err
	}
	raw, err := json.Marshal(full)
	if err != nil {
		return Output{}, err
	}
	revision, err := store.Save(ctx, snap.DatabaseID, snap.Revision, raw)
	if err != nil {
		return Output{}, fmt.Errorf("workspace changed or save failed; retry after reading again: %w", err)
	}
	return Output{Message: "Draft saved; not published.", Revision: revision}, nil
}
func newServer(store *workspace.Store) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "gitme-post-drafts", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_projects", Description: "List project IDs and names only."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []Project, error) {
		_, state, err := read(ctx, store)
		return nil, state.Projects, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "read_post_context", Description: "Read only the post context explicitly saved in Git/Me for the requested project."}, func(ctx context.Context, _ *mcp.CallToolRequest, in ProjectInput) (*mcp.CallToolResult, PostContext, error) {
		_, state, err := read(ctx, store)
		if err != nil {
			return nil, PostContext{}, err
		}
		if strings.TrimSpace(in.ProjectID) == "" {
			return nil, PostContext{}, errors.New("project ID required")
		}
		for _, item := range state.Contexts {
			if item.ProjectID == in.ProjectID {
				return nil, item, nil
			}
		}
		return nil, PostContext{}, errors.New("no saved post context for this project")
	})
	mcp.AddTool(server, &mcp.Tool{Name: "read_project_drafts", Description: "Read saved post drafts for the specified project."}, func(ctx context.Context, _ *mcp.CallToolRequest, in ProjectInput) (*mcp.CallToolResult, []Draft, error) {
		_, state, err := read(ctx, store)
		out := []Draft{}
		for _, d := range state.Drafts {
			if d.Project == in.ProjectID {
				out = append(out, d)
			}
		}
		return nil, out, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "create_post_draft", Description: "Save a post draft for a project. Does not publish."}, func(ctx context.Context, _ *mcp.CallToolRequest, in DraftInput) (*mcp.CallToolResult, Output, error) {
		out, err := saveDraft(ctx, store, Draft{Project: in.ProjectID, Platform: in.Platform, Text: in.Text}, nil)
		return nil, out, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "update_post_draft", Description: "Replace the text of a saved draft only if the expected text matches. Does not publish."}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateInput) (*mcp.CallToolResult, Output, error) {
		out, err := saveDraft(ctx, store, Draft{ID: in.ID, Text: in.Text}, &in.ExpectedText)
		return nil, out, err
	})
	return server
}

func main() {
	path, err := workspace.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}
	store, err := workspace.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	server := newServer(store)
	if err = server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
