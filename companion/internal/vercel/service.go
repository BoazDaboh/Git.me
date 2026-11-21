package vercel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitme/companion/internal/credentials"
)

type Account struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}
type Connection struct {
	Account
	Connected bool      `json:"connected"`
	Stored    bool      `json:"stored"`
	CheckedAt time.Time `json:"checked_at"`
	Message   string    `json:"message"`
}
type Team struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}
type Pagination struct {
	Next any `json:"next"`
}

var cursorPattern = regexp.MustCompile(`^[A-Za-z0-9_=.-]+$`)

func validPagination(p Pagination) bool {
	switch value := p.Next.(type) {
	case nil:
		return true
	case float64:
		return value > 0 && value <= 9007199254740991 && math.Trunc(value) == value
	case string:
		return len(value) > 0 && len(value) <= 4096 && cursorPattern.MatchString(value)
	}
	return false
}

type TeamsPage struct {
	Teams      []Team     `json:"teams"`
	Pagination Pagination `json:"pagination"`
}
type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AccountID  string `json:"accountId"`
	Framework  string `json:"framework"`
	Repository string `json:"repository,omitempty"`
}
type ProjectsPage struct {
	Projects   []Project  `json:"projects"`
	Pagination Pagination `json:"pagination"`
}
type Link struct {
	ConnectionID string `json:"connectionId"`
	ProjectID    string `json:"projectId"`
	ProjectName  string `json:"projectName"`
	TeamID       string `json:"teamId"`
	ScopeSlug    string `json:"scopeSlug"`
}
type Deployment struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	State     string `json:"state"`
	Target    string `json:"target"`
	CreatedAt int64  `json:"created_at"`
	Branch    string `json:"branch"`
	SHA       string `json:"sha"`
}
type Activity struct {
	Project     Project      `json:"project"`
	Production  *Deployment  `json:"production"`
	Deployments []Deployment `json:"deployments"`
	Warnings    []string     `json:"warnings"`
	CheckedAt   time.Time    `json:"checked_at"`
}
type connectionCache struct {
	Connection Connection
	Key        [32]byte
}
type activityCache struct{ Activity Activity }
type Service struct {
	mu         sync.Mutex
	index      credentials.Vault
	factory    func(string) credentials.Vault
	client     *http.Client
	loaded     bool
	accounts   []Account
	checks     map[string]connectionCache
	activity   map[string]activityCache
	tracking   map[string]CommitTracking
	generation uint64
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
var slug = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func ValidID(id string) bool { return identifier.MatchString(id) }
func ValidLink(link Link) bool {
	return ValidID(link.ConnectionID) && ValidID(link.ProjectID) && strings.HasPrefix(link.ProjectID, "prj_") && slug.MatchString(link.ProjectName) && (link.TeamID == "" || ValidID(link.TeamID)) && slug.MatchString(link.ScopeSlug)
}
func ValidateLink(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 5 {
		return errors.New("invalid Vercel project link")
	}
	for _, key := range []string{"connectionId", "projectId", "projectName", "teamId", "scopeSlug"} {
		var text *string
		if json.Unmarshal(fields[key], &text) != nil || text == nil {
			return errors.New("invalid Vercel project link")
		}
	}
	var link Link
	if json.Unmarshal(raw, &link) != nil || !ValidLink(link) {
		return errors.New("invalid Vercel project link")
	}
	return nil
}
func New(index credentials.Vault, factory func(string) credentials.Vault, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Service{index: index, factory: factory, client: client, checks: map[string]connectionCache{}, activity: map[string]activityCache{}, tracking: map[string]CommitTracking{}}
}
func (s *Service) load() error {
	if s.loaded {
		return nil
	}
	raw, err := s.index.Read()
	if errors.Is(err, credentials.ErrMissing) {
		s.accounts = []Account{}
	} else if err != nil {
		return err
	} else {
		var accounts []Account
		if json.Unmarshal([]byte(raw), &accounts) != nil || len(accounts) > 10 {
			return errors.New("Vercel account list could not be read")
		}
		seen := map[string]bool{}
		for _, a := range accounts {
			if !ValidID(a.ID) || !slug.MatchString(a.Username) || seen[a.ID] {
				return errors.New("Vercel account list could not be read")
			}
			seen[a.ID] = true
		}
		s.accounts = accounts
	}
	s.loaded = true
	return nil
}
func (s *Service) persist(accounts []Account) error {
	raw, err := json.Marshal(accounts)
	if err != nil {
		return errors.New("Vercel account list could not be saved")
	}
	return s.index.Write(string(raw))
}
func (s *Service) get(ctx context.Context, token, path string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.vercel.com"+path, nil)
	if err != nil {
		return errors.New("invalid Vercel request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "GitMe-local/0.7")
	response, err := s.client.Do(req)
	if err != nil {
		return errors.New("Vercel could not be reached; check your internet connection")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		switch response.StatusCode {
		case 401:
			return errors.New("Vercel rejected this token; it may be invalid, expired or revoked")
		case 403:
			return errors.New("Vercel access denied; check the token's scope and team permissions")
		case 404:
			return errors.New("Vercel project not found or this token cannot access it")
		case 429:
			return errors.New("Vercel rate limit reached; wait before refreshing")
		case 301, 302, 307, 308:
			return errors.New("Vercel resource moved; relink its current project")
		default:
			return fmt.Errorf("Vercel is temporarily unavailable (HTTP %d)", response.StatusCode)
		}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return errors.New("Vercel response exceeded the local read limit")
	}
	if json.Unmarshal(raw, result) != nil {
		return errors.New("Vercel returned an unexpected response")
	}
	return nil
}
func (s *Service) user(ctx context.Context, token string) (Account, error) {
	var response struct {
		User Account `json:"user"`
	}
	if err := s.get(ctx, token, "/v2/user", &response); err != nil {
		return Account{}, err
	}
	if !ValidID(response.User.ID) || !slug.MatchString(response.User.Username) {
		return Account{}, errors.New("Vercel account response could not be verified")
	}
	return response.User, nil
}
func (s *Service) Connect(ctx context.Context, token, replaceID string) (Connection, error) {
	token = strings.TrimSpace(token)
	if len(token) < 20 || len(token) > 2100 || !tokenPattern.MatchString(token) {
		return Connection{}, errors.New("paste the full Vercel access token, not its name or your password")
	}
	account, err := s.user(ctx, token)
	if err != nil {
		return Connection{}, err
	}
	if replaceID != "" && replaceID != account.ID {
		return Connection{}, errors.New("this token belongs to another account; use Add account instead")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = s.load(); err != nil {
		return Connection{}, err
	}
	position := -1
	for i, a := range s.accounts {
		if a.ID == account.ID {
			position = i
			break
		}
	}
	if replaceID != "" && position < 0 {
		return Connection{}, errors.New("choose a saved Vercel account")
	}
	if position < 0 && len(s.accounts) >= 10 {
		return Connection{}, errors.New("up to ten Vercel accounts can be saved on this PC")
	}
	next := append([]Account(nil), s.accounts...)
	if position < 0 {
		next = append(next, account)
	} else {
		next[position] = account
	}
	vault := s.factory(account.ID)
	previous, readErr := vault.Read()
	if readErr != nil && !errors.Is(readErr, credentials.ErrMissing) {
		return Connection{}, readErr
	}
	if err = vault.Write(token); err != nil {
		return Connection{}, err
	}
	if err = s.persist(next); err != nil {
		var rollbackErr error
		if readErr == nil {
			rollbackErr = vault.Write(previous)
		} else {
			rollbackErr = vault.Delete()
		}
		if rollbackErr != nil {
			return Connection{}, errors.New("Vercel credential storage failed; check this connection before retrying")
		}
		return Connection{}, err
	}
	connection := Connection{Account: account, Connected: true, Stored: true, CheckedAt: time.Now().UTC(), Message: "Connected"}
	s.accounts = next
	s.generation++
	s.checks[account.ID] = connectionCache{connection, sha256.Sum256([]byte(token))}
	s.clearActivity(account.ID)
	return connection, nil
}
func (s *Service) clearActivity(id string) {
	for key := range s.tracking {
		if strings.HasPrefix(key, id+"|") {
			delete(s.tracking, key)
		}
	}
	for key := range s.activity {
		if strings.HasPrefix(key, id+"|") {
			delete(s.activity, key)
		}
	}
}
func (s *Service) Disconnect(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return err
	}
	for i, a := range s.accounts {
		if a.ID != id {
			continue
		}
		if err := s.factory(id).Delete(); err != nil {
			return err
		}
		s.generation++
		delete(s.checks, id)
		s.clearActivity(id)
		next := append([]Account(nil), s.accounts[:i]...)
		next = append(next, s.accounts[i+1:]...)
		if err := s.persist(next); err != nil {
			return err
		}
		s.accounts = next
		return nil
	}
	return errors.New("choose a saved Vercel account")
}
func (s *Service) token(id string) (string, Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return "", Account{}, err
	}
	for _, a := range s.accounts {
		if a.ID == id {
			token, err := s.factory(id).Read()
			if errors.Is(err, credentials.ErrMissing) {
				err = errors.New("this Vercel account is disconnected; connect it in Settings")
			}
			return token, a, err
		}
	}
	return "", Account{}, errors.New("this Vercel account is disconnected; connect it in Settings")
}
func (s *Service) unchanged(id, token string) error {
	current, _, err := s.token(id)
	if err != nil || current != token {
		return errors.New("Vercel connection changed; refresh it")
	}
	return nil
}
func (s *Service) check(ctx context.Context, a Account, refresh bool) Connection {
	token, _, err := s.token(a.ID)
	if err != nil {
		return Connection{Account: a, Message: err.Error()}
	}
	key := sha256.Sum256([]byte(token))
	s.mu.Lock()
	cached := s.checks[a.ID]
	s.mu.Unlock()
	age := time.Since(cached.Connection.CheckedAt)
	if cached.Key == key && age < time.Minute && (!refresh || age < 10*time.Second) {
		return cached.Connection
	}
	account, err := s.user(ctx, token)
	result := Connection{Account: a, Stored: true, CheckedAt: time.Now().UTC(), Message: "Connected"}
	if err != nil {
		result.Message = err.Error()
	} else if account.ID != a.ID {
		result.Message = "Vercel account identity changed; replace its token"
	} else {
		result.Account = account
		result.Connected = true
	}
	if s.unchanged(a.ID, token) != nil {
		return Connection{Account: a, Message: "Vercel connection changed; refresh it"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, readErr := s.factory(a.ID).Read()
	if readErr == nil && current == token {
		s.checks[a.ID] = connectionCache{result, key}
	}
	return result
}
func (s *Service) Connections(ctx context.Context, refresh bool) ([]Connection, error) {
	s.mu.Lock()
	if err := s.load(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	accounts := append([]Account(nil), s.accounts...)
	generation := s.generation
	s.mu.Unlock()
	results := make([]Connection, len(accounts))
	var wg sync.WaitGroup
	for i, a := range accounts {
		wg.Add(1)
		go func(i int, a Account) { defer wg.Done(); results[i] = s.check(ctx, a, refresh) }(i, a)
	}
	wg.Wait()
	s.mu.Lock()
	changed := generation != s.generation
	s.mu.Unlock()
	if changed {
		return s.Connections(ctx, false)
	}
	output := []Connection{}
	for _, result := range results {
		if result.Stored {
			output = append(output, result)
		}
	}
	return output, nil
}
func scopeQuery(team string, until int64) (url.Values, error) {
	if (team != "" && !ValidID(team)) || until < 0 {
		return nil, errors.New("choose a valid Vercel scope or page")
	}
	q := url.Values{}
	if team != "" {
		q.Set("teamId", team)
	}
	if until > 0 {
		q.Set("until", strconv.FormatInt(until, 10))
	}
	return q, nil
}
func (s *Service) Teams(ctx context.Context, id string, until int64, from string) (TeamsPage, error) {
	q, err := scopeQuery("", until)
	if err != nil {
		return TeamsPage{}, err
	}
	q.Set("limit", "100")
	if from != "" {
		if until > 0 || len(from) > 4096 || !cursorPattern.MatchString(from) {
			return TeamsPage{}, errors.New("invalid Vercel team page")
		}
		q.Set("from", from)
	}
	token, _, err := s.token(id)
	if err != nil {
		return TeamsPage{}, err
	}
	var result TeamsPage
	if err = s.get(ctx, token, "/v2/teams?"+q.Encode(), &result); err != nil {
		return TeamsPage{}, err
	}
	for _, team := range result.Teams {
		if !ValidID(team.ID) || !slug.MatchString(team.Slug) {
			return TeamsPage{}, errors.New("Vercel returned an invalid team")
		}
	}
	if result.Teams == nil {
		result.Teams = []Team{}
	}
	if !validPagination(result.Pagination) {
		return TeamsPage{}, errors.New("Vercel returned an invalid team page")
	}
	return result, s.unchanged(id, token)
}
func owner(account Account, team string) string {
	if team != "" {
		return team
	}
	return account.ID
}
func (s *Service) Projects(ctx context.Context, id, team string, until int64, from, search string) (ProjectsPage, error) {
	q, err := scopeQuery(team, until)
	if err != nil {
		return ProjectsPage{}, err
	}
	if len(search) > 100 {
		return ProjectsPage{}, errors.New("project search is too long")
	}
	q.Set("limit", "20")
	if from != "" {
		if until > 0 || len(from) > 4096 || !cursorPattern.MatchString(from) {
			return ProjectsPage{}, errors.New("invalid Vercel project page")
		}
		q.Set("from", from)
	}
	if search != "" {
		q.Set("search", search)
	}
	token, account, err := s.token(id)
	if err != nil {
		return ProjectsPage{}, err
	}
	var raw json.RawMessage
	if err = s.get(ctx, token, "/v10/projects?"+q.Encode(), &raw); err != nil {
		return ProjectsPage{}, err
	}
	var result ProjectsPage
	if json.Unmarshal(raw, &result) != nil {
		if json.Unmarshal(raw, &result.Projects) != nil {
			return ProjectsPage{}, errors.New("Vercel returned an unexpected project list")
		}
	}
	for _, project := range result.Projects {
		if !ValidID(project.ID) || !strings.HasPrefix(project.ID, "prj_") || !slug.MatchString(project.Name) || project.AccountID != owner(account, team) {
			return ProjectsPage{}, errors.New("Vercel project scope could not be verified")
		}
	}
	if result.Projects == nil {
		result.Projects = []Project{}
	}
	if !validPagination(result.Pagination) {
		return ProjectsPage{}, errors.New("Vercel returned an invalid project page")
	}
	return result, s.unchanged(id, token)
}

type projectResponse struct {
	Project
	RootDirectory string `json:"rootDirectory"`
	Link          *struct {
		Type string `json:"type"`
		Org  string `json:"org"`
		Repo string `json:"repo"`
	} `json:"link"`
	Targets map[string]struct {
		ID         string `json:"id"`
		URL        string `json:"url"`
		ReadyState string `json:"readyState"`
		CreatedAt  int64  `json:"createdAt"`
	} `json:"targets"`
}

func (s *Service) project(ctx context.Context, token string, account Account, team, name string) (projectResponse, error) {
	q, err := scopeQuery(team, 0)
	if err != nil || !slug.MatchString(name) {
		return projectResponse{}, errors.New("choose a valid Vercel project")
	}
	var result projectResponse
	if err = s.get(ctx, token, "/v9/projects/"+url.PathEscape(name)+"?"+q.Encode(), &result); err != nil {
		return result, err
	}
	if !ValidID(result.ID) || !strings.HasPrefix(result.ID, "prj_") || !slug.MatchString(result.Name) || result.AccountID != owner(account, team) {
		return projectResponse{}, errors.New("Vercel project scope could not be verified")
	}
	if result.Link != nil && result.Link.Type == "github" && deploymentRepository.MatchString(result.Link.Org+"/"+result.Link.Repo) {
		result.Project.Repository = result.Link.Org + "/" + result.Link.Repo
	}
	return result, nil
}
func (s *Service) Project(ctx context.Context, id, team, name string) (Project, error) {
	token, a, err := s.token(id)
	if err != nil {
		return Project{}, err
	}
	result, err := s.project(ctx, token, a, team, name)
	if err != nil {
		return Project{}, err
	}
	return result.Project, s.unchanged(id, token)
}
func state(value string) string {
	switch value {
	case "READY", "BUILDING", "QUEUED", "INITIALIZING", "ERROR", "CANCELED", "BLOCKED":
		return value
	}
	return "UNKNOWN"
}
func deploymentURL(value string) string {
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !strings.HasSuffix(strings.ToLower(u.Hostname()), ".vercel.app") || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return ""
	}
	return u.String()
}
func bounded(value string, n int) string {
	r := []rune(value)
	if len(r) > n {
		return string(r[:n])
	}
	return value
}
func (s *Service) Activity(ctx context.Context, link Link, refresh bool) (Activity, error) {
	if !ValidLink(link) {
		return Activity{}, errors.New("choose a saved Vercel project")
	}
	token, a, err := s.token(link.ConnectionID)
	if err != nil {
		return Activity{}, err
	}
	key := fmt.Sprintf("%s|%s|%s|%x", a.ID, link.TeamID, link.ProjectID, sha256.Sum256([]byte(token)))
	s.mu.Lock()
	cached, exists := s.activity[key]
	s.mu.Unlock()
	age := time.Since(cached.Activity.CheckedAt)
	if exists && age < time.Minute && (!refresh || age < 10*time.Second) {
		return cached.Activity, nil
	}
	p, err := s.project(ctx, token, a, link.TeamID, link.ProjectID)
	if err != nil {
		return Activity{}, err
	}
	if p.ID != link.ProjectID {
		return Activity{}, errors.New("Vercel project identity changed; relink the project")
	}
	result := Activity{Project: p.Project, Deployments: []Deployment{}, Warnings: []string{}, CheckedAt: time.Now().UTC()}
	if current, ok := p.Targets["production"]; ok && ValidID(current.ID) {
		result.Production = &Deployment{ID: current.ID, URL: deploymentURL(current.URL), State: state(current.ReadyState), Target: "production", CreatedAt: current.CreatedAt}
	}
	q, _ := scopeQuery(link.TeamID, 0)
	q.Set("projectId", link.ProjectID)
	q.Set("limit", "6")
	var list struct {
		Deployments []struct {
			UID        string            `json:"uid"`
			URL        string            `json:"url"`
			State      string            `json:"state"`
			ReadyState string            `json:"readyState"`
			CreatedAt  int64             `json:"createdAt"`
			Created    int64             `json:"created"`
			Target     string            `json:"target"`
			Meta       map[string]string `json:"meta"`
		} `json:"deployments"`
	}
	if err = s.get(ctx, token, "/v7/deployments?"+q.Encode(), &list); err != nil {
		result.Warnings = append(result.Warnings, "Recent deployments: "+err.Error())
	} else {
		for _, d := range list.Deployments {
			if !ValidID(d.UID) {
				continue
			}
			if d.State == "" {
				d.State = d.ReadyState
			}
			if d.CreatedAt == 0 {
				d.CreatedAt = d.Created
			}
			target := d.Target
			if target == "" {
				target = "preview"
			}
			if target != "production" && target != "preview" {
				target = "custom"
			}
			branch, sha := "", ""
			for _, provider := range []string{"github", "gitlab", "bitbucket"} {
				if branch == "" {
					branch = d.Meta[provider+"CommitRef"]
				}
				if sha == "" {
					sha = d.Meta[provider+"CommitSha"]
				}
			}
			result.Deployments = append(result.Deployments, Deployment{ID: d.UID, URL: deploymentURL(d.URL), State: state(d.State), Target: target, CreatedAt: d.CreatedAt, Branch: bounded(branch, 100), SHA: bounded(sha, 64)})
		}
	}
	if err = s.unchanged(a.ID, token); err != nil {
		return Activity{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, readErr := s.factory(a.ID).Read()
	if readErr != nil || current != token {
		return Activity{}, errors.New("Vercel connection changed; refresh it")
	}
	if len(s.activity) >= 64 {
		for k := range s.activity {
			delete(s.activity, k)
			break
		}
	}
	s.activity[key] = activityCache{result}
	return result, nil
}
