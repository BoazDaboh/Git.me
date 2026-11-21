package workspace

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"gitme/companion/internal/github"
	"gitme/companion/internal/releases"
	"gitme/companion/internal/vercel"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrConflict = errors.New("workspace changed; reload before saving")

var tables = []string{"projects", "tasks", "notes", "sessions", "drafts"}
var githubName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)

func validateGitHub(raw json.RawMessage) error {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || (len(value) != 2 && len(value) != 3) {
		return errors.New("invalid GitHub repository link")
	}
	for key := range value {
		if key != "id" && key != "fullName" && key != "connectionId" {
			return errors.New("invalid GitHub repository link")
		}
	}
	if rawID, exists := value["connectionId"]; exists {
		var connectionID string
		if json.Unmarshal(rawID, &connectionID) != nil || !github.ValidConnectionID(connectionID) {
			return errors.New("invalid GitHub account link")
		}
	}
	var id int64
	var name string
	if json.Unmarshal(value["id"], &id) != nil || id <= 0 || id > 9007199254740991 || json.Unmarshal(value["fullName"], &name) != nil || !githubName.MatchString(name) {
		return errors.New("invalid GitHub repository link")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." {
			return errors.New("invalid GitHub repository link")
		}
	}
	return nil
}

type Snapshot struct {
	ServiceLinkMigration bool            `json:"service_link_migration,omitempty"`
	DatabaseID           string          `json:"database_id"`
	Revision             int64           `json:"revision"`
	UpdatedAt            string          `json:"updated_at"`
	Workspace            json.RawMessage `json:"workspace"`
}

type Store struct {
	db   *sql.DB
	Path string
}

func DefaultPath() (string, error) {
	if directory := os.Getenv("GITME_DATA_DIR"); directory != "" {
		absolute, err := filepath.Abs(directory)
		return filepath.Join(absolute, "gitme.sqlite"), err
	}
	directory, err := os.UserConfigDir()
	return filepath.Join(directory, "GitMe", "data", "gitme.sqlite"), err
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, Path: path}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	for _, statement := range []string{"PRAGMA busy_timeout = 5000", "PRAGMA journal_mode = DELETE", "PRAGMA synchronous = FULL"} {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 3 {
		return errors.New("database schema is newer than this companion")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS workspace_state (
		id INTEGER PRIMARY KEY CHECK (id = 1), database_id TEXT NOT NULL,
		revision INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL DEFAULT '',
		settings TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(settings)))`); err != nil {
		return err
	}
	for _, table := range tables {
		if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS ` + table + ` (
			id TEXT PRIMARY KEY, position INTEGER NOT NULL,
			payload TEXT NOT NULL CHECK (json_valid(payload)))`); err != nil {
			return err
		}
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT OR IGNORE INTO workspace_state (id, database_id) VALUES (1, ?)", fmt.Sprintf("%x", id)); err != nil {
		return err
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS release_records (id TEXT PRIMARY KEY,project_id TEXT NOT NULL,service_id TEXT NOT NULL,created_at TEXT NOT NULL,payload TEXT NOT NULL CHECK(json_valid(payload)))`); err != nil {
		return err
	}
	if _, err = tx.Exec(`CREATE INDEX IF NOT EXISTS release_service_history ON release_records(project_id,service_id,created_at DESC)`); err != nil {
		return err
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS deployment_actions (id TEXT PRIMARY KEY,project_id TEXT NOT NULL,service_id TEXT NOT NULL,created_at TEXT NOT NULL,payload TEXT NOT NULL CHECK(json_valid(payload)))`); err != nil {
		return err
	}
	if _, err = tx.Exec(`CREATE INDEX IF NOT EXISTS deployment_action_history ON deployment_actions(project_id,service_id,created_at DESC)`); err != nil {
		return err
	}
	if _, err = tx.Exec("PRAGMA user_version = 3"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Load(ctx context.Context) (Snapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()
	var result Snapshot
	var settings string
	if err = tx.QueryRowContext(ctx, "SELECT database_id, revision, updated_at, settings FROM workspace_state WHERE id = 1").Scan(&result.DatabaseID, &result.Revision, &result.UpdatedAt, &settings); err != nil {
		return result, err
	}
	if result.Revision == 0 {
		result.Workspace = json.RawMessage("null")
		return result, tx.Commit()
	}
	var data map[string]json.RawMessage
	if err = json.Unmarshal([]byte(settings), &data); err != nil {
		return result, err
	}
	for _, table := range tables {
		rows, queryErr := tx.QueryContext(ctx, "SELECT payload FROM "+table+" ORDER BY position")
		if queryErr != nil {
			return result, queryErr
		}
		items := []json.RawMessage{}
		for rows.Next() {
			var payload string
			if err = rows.Scan(&payload); err != nil {
				rows.Close()
				return result, err
			}
			items = append(items, json.RawMessage(payload))
		}
		readErr := rows.Err()
		rows.Close()
		if readErr != nil {
			return result, readErr
		}
		data[table], err = json.Marshal(items)
		if err != nil {
			return result, err
		}
	}
	result.Workspace, err = json.Marshal(data)
	if err != nil {
		return result, err
	}
	result.Workspace, result.ServiceLinkMigration, err = migrateProjectGitHub(result.Workspace)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func Validate(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil || data == nil {
		return nil, errors.New("workspace must be an object")
	}
	if string(data["version"]) != "1" {
		return nil, errors.New("unsupported workspace version")
	}
	for _, table := range tables {
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(data[table], &items); err != nil || items == nil || len(items) > 10000 {
			return nil, fmt.Errorf("invalid %s collection", table)
		}
		seen := map[string]bool{}
		for _, item := range items {
			if item == nil {
				return nil, fmt.Errorf("invalid %s record", table)
			}
			if table != "sessions" {
				var id string
				if json.Unmarshal(item["id"], &id) != nil || id == "" || seen[id] {
					return nil, fmt.Errorf("missing or duplicate %s id", table)
				}
				seen[id] = true
			}
			required := map[string][]string{"projects": {"name", "initials", "color", "description", "stage", "branch", "folder", "url", "local", "note"}, "tasks": {"title", "project", "status", "priority"}, "notes": {"title", "body", "kind", "project"}, "drafts": {"platform", "project", "text"}, "sessions": {"project"}}[table]
			for _, field := range required {
				var value string
				if json.Unmarshal(item[field], &value) != nil || string(item[field]) == "null" {
					return nil, fmt.Errorf("invalid %s.%s", table, field)
				}
			}
			if table == "projects" {
				if raw, exists := item["apiRequests"]; exists {
					if err := validateAPIRequests(raw); err != nil {
						return nil, err
					}
				}
				if raw, exists := item["unassignedGitHubLinks"]; exists {
					var links []json.RawMessage
					if json.Unmarshal(raw, &links) != nil || links == nil || len(links) > 100 {
						return nil, errors.New("invalid saved repository links")
					}
					for _, link := range links {
						if err := validateGitHub(link); err != nil {
							return nil, err
						}
					}
				}
				if raw, exists := item["vercel"]; exists {
					if err := vercel.ValidateLink(raw); err != nil {
						return nil, err
					}
				}
				if raw, exists := item["github"]; exists {
					if err := validateGitHub(raw); err != nil {
						return nil, err
					}
				}
				var stack []string
				var progress float64
				if json.Unmarshal(item["stack"], &stack) != nil || stack == nil || json.Unmarshal(item["progress"], &progress) != nil || string(item["progress"]) == "null" || progress < 0 || progress > 100 {
					return nil, errors.New("invalid project stack or progress")
				}
				if profiles, exists := item["services"]; exists {
					if err := validateServices(profiles); err != nil {
						return nil, err
					}
				}
			}
			if table == "notes" {
				var pinned bool
				var updated float64
				var kind string
				_ = json.Unmarshal(item["kind"], &kind)
				if json.Unmarshal(item["pinned"], &pinned) != nil || string(item["pinned"]) == "null" || json.Unmarshal(item["updatedAt"], &updated) != nil || updated <= 0 || (kind != "note" && kind != "plan") {
					return nil, errors.New("invalid note metadata")
				}
			}
			if table == "tasks" {
				var status string
				_ = json.Unmarshal(item["status"], &status)
				if status != "Planned" && status != "In progress" && status != "Done" {
					return nil, errors.New("invalid task status")
				}
			}
			if table == "sessions" {
				var start, end float64
				if json.Unmarshal(item["started"], &start) != nil || json.Unmarshal(item["ended"], &end) != nil || start <= 0 || end < start {
					return nil, errors.New("invalid session times")
				}
			}
		}
	}
	var document struct {
		Title *string `json:"title"`
		Text  *string `json:"text"`
	}
	if json.Unmarshal(data["document"], &document) != nil || document.Title == nil || document.Text == nil {
		return nil, errors.New("invalid document")
	}
	if string(data["focus"]) != "null" {
		var focus struct {
			Project *string `json:"project"`
			Started float64 `json:"started"`
		}
		if json.Unmarshal(data["focus"], &focus) != nil || focus.Project == nil || focus.Started <= 0 {
			return nil, errors.New("invalid active focus session")
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(data["focus"], &fields) != nil {
			return nil, errors.New("invalid active focus session")
		}
		if raw, exists := fields["elapsedMs"]; exists {
			var elapsed int64
			if string(raw) == "null" || json.Unmarshal(raw, &elapsed) != nil || elapsed < 0 || elapsed > 9007199254740991 {
				return nil, errors.New("invalid saved focus duration")
			}
		}
	}
	if raw, exists := data["projectDirectory"]; exists {
		var directory *string
		if json.Unmarshal(raw, &directory) != nil || directory == nil {
			return nil, errors.New("invalid project directory")
		}
	}
	if raw, exists := data["collapsedTaskGroups"]; exists {
		var groups []string
		if json.Unmarshal(raw, &groups) != nil || groups == nil {
			return nil, errors.New("invalid collapsed task groups")
		}
	}
	return data, nil
}

func validateServices(raw json.RawMessage) error {
	var profiles []map[string]json.RawMessage
	if json.Unmarshal(raw, &profiles) != nil || profiles == nil {
		return errors.New("invalid service profiles")
	}
	seen := map[string]bool{}
	for _, profile := range profiles {
		if raw, exists := profile["release"]; exists {
			if err := releases.Validate(raw); err != nil {
				return err
			}
		}
		if raw, exists := profile["vercel"]; exists {
			if err := vercel.ValidateLink(raw); err != nil {
				return err
			}
		}
		if raw, exists := profile["github"]; exists {
			if err := validateGitHub(raw); err != nil {
				return err
			}
		}
		for _, field := range []string{"id", "name", "role", "runtime", "folder", "repository", "branch", "healthPath"} {
			var value *string
			if json.Unmarshal(profile[field], &value) != nil || value == nil {
				return errors.New("invalid service field: " + field)
			}
			if field == "id" {
				if *value == "" || seen[*value] {
					return errors.New("missing or duplicate service id")
				}
				seen[*value] = true
			}
			if field == "role" && *value != "frontend" && *value != "backend" && *value != "worker" && *value != "database" && *value != "other" {
				return errors.New("invalid service role")
			}
		}
		var stack []string
		var port *int
		var updated float64
		if json.Unmarshal(profile["stack"], &stack) != nil || stack == nil || json.Unmarshal(profile["port"], &port) != nil || (port != nil && (*port < 1 || *port > 65535)) || json.Unmarshal(profile["updatedAt"], &updated) != nil || updated <= 0 {
			return errors.New("invalid service stack, port or timestamp")
		}
		for group, fields := range map[string][]string{"commands": {"install", "develop", "build", "test", "start"}, "deployment": {"provider", "method", "build", "start", "dockerfile", "image"}} {
			var values map[string]json.RawMessage
			if json.Unmarshal(profile[group], &values) != nil {
				return errors.New("invalid service " + group)
			}
			for _, field := range fields {
				var value *string
				if json.Unmarshal(values[field], &value) != nil || value == nil {
					return errors.New("invalid service " + group + "." + field)
				}
			}
		}
		var environments map[string]json.RawMessage
		if json.Unmarshal(profile["environments"], &environments) != nil {
			return errors.New("invalid service environments")
		}
		for _, name := range []string{"local", "preview", "production"} {
			var config struct {
				URL       *string  `json:"url"`
				Variables []string `json:"variables"`
			}
			if json.Unmarshal(environments[name], &config) != nil || config.URL == nil || config.Variables == nil {
				return errors.New("invalid service environment: " + name)
			}
		}
		if raw, exists := profile["commandOverrides"]; exists {
			var commands []string
			if json.Unmarshal(raw, &commands) != nil || commands == nil {
				return errors.New("invalid command overrides")
			}
			for _, command := range commands {
				if command != "install" && command != "develop" && command != "build" && command != "test" && command != "start" {
					return errors.New("invalid command override")
				}
			}
		}
		if raw, exists := profile["detection"]; exists {
			var detected struct {
				Key       *string  `json:"key"`
				Root      *string  `json:"root"`
				ScannedAt *string  `json:"scannedAt"`
				Files     []string `json:"files"`
			}
			if json.Unmarshal(raw, &detected) != nil || detected.Key == nil || detected.Root == nil || detected.ScannedAt == nil || detected.Files == nil {
				return errors.New("invalid scan provenance")
			}
		}
	}
	return nil
}

func (s *Store) Save(ctx context.Context, databaseID string, revision int64, raw json.RawMessage) (int64, error) {
	data, err := Validate(raw)
	if err != nil {
		return 0, err
	}
	if normalized, changed, migrationErr := migrateProjectGitHub(raw); migrationErr != nil {
		return 0, migrationErr
	} else if changed {
		data, err = Validate(normalized)
		if err != nil {
			return 0, err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	updated := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, "UPDATE workspace_state SET revision = revision + 1, updated_at = ? WHERE id = 1 AND database_id = ? AND revision = ?", updated, databaseID, revision)
	if err != nil {
		return 0, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if changed != 1 {
		return 0, ErrConflict
	}
	for _, table := range tables {
		var items []json.RawMessage
		if err = json.Unmarshal(data[table], &items); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return 0, err
		}
		for position, payload := range items {
			id := fmt.Sprint(position)
			if table != "sessions" {
				var item struct {
					ID string `json:"id"`
				}
				if err = json.Unmarshal(payload, &item); err != nil {
					return 0, err
				}
				id = item.ID
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO "+table+" (id, position, payload) VALUES (?, ?, ?)", id, position, string(payload)); err != nil {
				return 0, err
			}
		}
		delete(data, table)
	}
	settings, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE workspace_state SET settings = ? WHERE id = 1", string(settings)); err != nil {
		return 0, err
	}
	return revision + 1, tx.Commit()
}
