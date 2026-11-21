package github

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Login string `json:"login"`
	Name  string `json:"name,omitempty"`
	ID    int64  `json:"id"`
}
type Connection struct {
	Connected bool      `json:"connected"`
	Stored    bool      `json:"stored"`
	Account   *Account  `json:"account,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	Message   string    `json:"message"`
}
type Repository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	PushedAt      string `json:"pushed_at"`
	Archived      bool   `json:"archived"`
}
type Page struct {
	Repositories []Repository `json:"repositories"`
	Next         bool         `json:"next"`
	Page         int          `json:"page"`
}
type Branch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}
type Commit struct {
	SHA    string `json:"sha"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Date   string `json:"date"`
}
type Pull struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Draft  bool   `json:"draft"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
	UpdatedAt string `json:"updated_at"`
}
type Activity struct {
	Repository Repository `json:"repository"`
	Branches   []Branch   `json:"branches"`
	Commits    []Commit   `json:"commits"`
	Pulls      []Pull     `json:"pulls"`
	Warnings   []string   `json:"warnings"`
	CheckedAt  time.Time  `json:"checked_at"`
}
type cached struct{ data Activity }
type Service struct {
	vault         credentials.Vault
	client        *http.Client
	mu            sync.Mutex
	connection    Connection
	connectionKey [32]byte
	cache         map[string]cached
}

func New(vault credentials.Vault, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Service{vault: vault, client: client, cache: map[string]cached{}}
}

var tokenPattern = regexp.MustCompile(`^(github_pat_|ghp_)[A-Za-z0-9_]{20,}$`)
var segment = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

func ValidRepository(name string) bool {
	parts := strings.Split(name, "/")
	return len(parts) == 2 && segment.MatchString(parts[0]) && segment.MatchString(parts[1]) && parts[0] != "." && parts[0] != ".." && parts[1] != "." && parts[1] != ".."
}
func shorten(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func (s *Service) get(ctx context.Context, token, path string, result any) (http.Header, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com"+path, nil)
	if err != nil {
		return nil, errors.New("invalid GitHub request")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	request.Header.Set("User-Agent", "GitMe-local/0.5")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, errors.New("GitHub could not be reached; check your internet connection")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		switch response.StatusCode {
		case 401:
			return nil, errors.New("GitHub rejected this token; it may be invalid, expired or revoked")
		case 403, 429:
			if response.Header.Get("X-RateLimit-Remaining") == "0" || response.StatusCode == 429 {
				return nil, errors.New("GitHub rate limit reached; wait before refreshing")
			}
			return nil, errors.New("GitHub access denied; check repository selection, read permissions and any organisation approval")
		case 404:
			return nil, errors.New("repository not found or this token cannot access it")
		case 409:
			return nil, errors.New("repository is empty or this activity is not available yet")
		case 301, 302, 307, 308:
			return nil, errors.New("GitHub resource moved; select its current repository name")
		default:
			return nil, fmt.Errorf("GitHub is temporarily unavailable (HTTP %d)", response.StatusCode)
		}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return nil, errors.New("GitHub response exceeded the local read limit")
	}
	if json.Unmarshal(data, result) != nil {
		return nil, errors.New("GitHub returned an unexpected response")
	}
	return response.Header, nil
}

func (s *Service) Connect(ctx context.Context, token string) (Connection, error) {
	token = strings.TrimSpace(token)
	if len(token) > 2100 || !tokenPattern.MatchString(token) {
		return Connection{}, errors.New("enter a GitHub personal access token, not an account password")
	}
	var account Account
	if _, err := s.get(ctx, token, "/user", &account); err != nil {
		return Connection{}, err
	}
	if account.ID <= 0 || account.ID > 9007199254740991 || account.Login == "" || !segment.MatchString(account.Login) {
		return Connection{}, errors.New("GitHub account response could not be verified")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.vault.Write(token); err != nil {
		return Connection{}, err
	}
	s.cache = map[string]cached{}
	s.connectionKey = sha256.Sum256([]byte(token))
	s.connection = Connection{Connected: true, Stored: true, Account: &account, CheckedAt: time.Now().UTC(), Message: "Connected"}
	return s.connection, nil
}
func (s *Service) Connection(ctx context.Context, refresh bool) (Connection, error) {
	s.mu.Lock()
	token, err := s.vault.Read()
	if errors.Is(err, credentials.ErrMissing) {
		s.mu.Unlock()
		return Connection{Message: "Not connected"}, nil
	}
	if err != nil {
		s.mu.Unlock()
		return Connection{}, err
	}
	key := sha256.Sum256([]byte(token))
	age := time.Since(s.connection.CheckedAt)
	if key == s.connectionKey && age < time.Minute && (!refresh || age < 10*time.Second) {
		result := s.connection
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()
	var account Account
	_, err = s.get(ctx, token, "/user", &account)
	result := Connection{Stored: true, CheckedAt: time.Now().UTC(), Message: "Connected"}
	if err != nil {
		result.Message = err.Error()
	} else if account.ID <= 0 || account.ID > 9007199254740991 || !segment.MatchString(account.Login) {
		result.Message = "GitHub account response could not be verified"
	} else {
		result.Connected = true
		result.Account = &account
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, readErr := s.vault.Read()
	if errors.Is(readErr, credentials.ErrMissing) {
		return Connection{Message: "Not connected"}, nil
	}
	if readErr != nil || sha256.Sum256([]byte(current)) != key {
		return Connection{Stored: readErr == nil, Message: "GitHub connection changed; refresh it"}, nil
	}
	s.connection = result
	s.connectionKey = key
	return result, nil
}
func (s *Service) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.vault.Delete(); err != nil {
		return err
	}
	s.cache = map[string]cached{}
	s.connection = Connection{}
	s.connectionKey = [32]byte{}
	return nil
}
func (s *Service) token() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, err := s.vault.Read()
	if errors.Is(err, credentials.ErrMissing) {
		return "", errors.New("connect GitHub in Settings first")
	}
	return token, err
}
func (s *Service) unchanged(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.vault.Read()
	if err != nil || current != token {
		return errors.New("GitHub connection changed; refresh it")
	}
	return nil
}

func (s *Service) Repositories(ctx context.Context, page int) (Page, error) {
	if page < 1 || page > 1000 {
		return Page{}, errors.New("invalid repository page")
	}
	token, err := s.token()
	if err != nil {
		return Page{}, err
	}
	result := Page{Repositories: []Repository{}, Page: page}
	headers, err := s.get(ctx, token, "/user/repos?sort=updated&direction=desc&per_page=30&page="+strconv.Itoa(page), &result.Repositories)
	if err != nil {
		return result, err
	}
	for _, repo := range result.Repositories {
		if repo.ID <= 0 || !ValidRepository(repo.FullName) {
			return Page{}, errors.New("GitHub returned invalid repository details")
		}
	}
	if result.Repositories == nil {
		result.Repositories = []Repository{}
	}
	result.Next = strings.Contains(headers.Get("Link"), `rel="next"`)
	if err := s.unchanged(token); err != nil {
		return Page{}, err
	}
	return result, nil
}

func (s *Service) Repository(ctx context.Context, name string) (Repository, error) {
	if !ValidRepository(name) {
		return Repository{}, errors.New("use a repository name like owner/repository")
	}
	token, err := s.token()
	if err != nil {
		return Repository{}, err
	}
	result, err := s.repository(ctx, token, name)
	if err != nil {
		return result, err
	}
	if err = s.unchanged(token); err != nil {
		return Repository{}, err
	}
	return result, nil
}
func (s *Service) repository(ctx context.Context, token, name string) (Repository, error) {
	var repo Repository
	_, err := s.get(ctx, token, "/repos/"+name, &repo)
	if err != nil {
		return repo, err
	}
	if repo.ID <= 0 || !ValidRepository(repo.FullName) {
		return repo, errors.New("GitHub returned invalid repository details")
	}
	return repo, nil
}

func (s *Service) Activity(ctx context.Context, name string, id int64, refresh bool) (Activity, error) {
	if !ValidRepository(name) {
		return Activity{}, errors.New("linked repository is invalid")
	}
	token, err := s.token()
	if err != nil {
		return Activity{}, err
	}
	fingerprint := sha256.Sum256([]byte(token))
	key := fmt.Sprintf("%x:%s", fingerprint, strings.ToLower(name))
	s.mu.Lock()
	old, exists := s.cache[key]
	if exists {
		age := time.Since(old.data.CheckedAt)
		if old.data.Repository.ID == id && age < time.Minute && (!refresh || age < 10*time.Second) {
			s.mu.Unlock()
			return old.data, nil
		}
	}
	s.mu.Unlock()
	repo, err := s.repository(ctx, token, name)
	if err != nil {
		return Activity{}, err
	}
	if repo.ID != id {
		return Activity{}, errors.New("repository identity changed; verify and link it again")
	}
	activity := Activity{Repository: repo, Branches: []Branch{}, Commits: []Commit{}, Pulls: []Pull{}, Warnings: []string{}, CheckedAt: time.Now().UTC()}
	var branches []Branch
	var pulls []Pull
	var commits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	var branchErr, commitErr, pullErr error
	var group sync.WaitGroup
	group.Add(3)
	go func() {
		defer group.Done()
		_, branchErr = s.get(ctx, token, "/repos/"+name+"/branches?per_page=10", &branches)
	}()
	go func() {
		defer group.Done()
		_, commitErr = s.get(ctx, token, "/repos/"+name+"/commits?per_page=5&sha="+url.QueryEscape(repo.DefaultBranch), &commits)
	}()
	go func() {
		defer group.Done()
		_, pullErr = s.get(ctx, token, "/repos/"+name+"/pulls?state=open&sort=updated&direction=desc&per_page=5", &pulls)
	}()
	group.Wait()
	if branchErr != nil {
		activity.Warnings = append(activity.Warnings, "Branches: "+branchErr.Error())
	} else {
		activity.Branches = branches
		if activity.Branches == nil {
			activity.Branches = []Branch{}
		}
	}
	if pullErr != nil {
		activity.Warnings = append(activity.Warnings, "Pull requests: "+pullErr.Error())
	} else {
		for _, pull := range pulls {
			if pull.Number > 0 {
				pull.Title = shorten(pull.Title, 240)
				activity.Pulls = append(activity.Pulls, pull)
			}
		}
	}
	if commitErr != nil {
		activity.Warnings = append(activity.Warnings, "Commits: "+commitErr.Error())
	} else {
		for _, commit := range commits {
			if regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`).MatchString(commit.SHA) {
				activity.Commits = append(activity.Commits, Commit{SHA: commit.SHA, Title: shorten(strings.SplitN(commit.Commit.Message, "\n", 2)[0], 180), Author: shorten(commit.Commit.Author.Name, 100), Date: commit.Commit.Author.Date})
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, readErr := s.vault.Read()
	if readErr != nil || sha256.Sum256([]byte(current)) != fingerprint {
		return Activity{}, errors.New("GitHub connection changed; refresh it")
	}
	if len(s.cache) >= 64 {
		for k := range s.cache {
			delete(s.cache, k)
			break
		}
	}
	s.cache[key] = cached{data: activity}
	return activity, nil
}
