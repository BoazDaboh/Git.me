package processes

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Health struct {
	URL        string `json:"url"`
	Status     int    `json:"status"`
	Responding bool   `json:"responding"`
	Duration   int64  `json:"duration_ms"`
	Message    string `json:"message"`
}

func (m *Manager) Health(ctx context.Context, project, service string) (Health, error) {
	config, err := m.config(ctx, project, service)
	if err != nil {
		return Health{}, err
	}
	base := config.LocalURL
	if base == "" && config.Port != nil {
		base = "http://127.0.0.1:" + strconv.Itoa(*config.Port)
	}
	u, err := url.Parse(base)
	if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return Health{}, errors.New("set a local HTTP URL or port in the service profile")
	}
	host := strings.ToLower(u.Hostname())
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return Health{}, errors.New("health checks only support loopback URLs on this PC")
	}
	if config.HealthPath != "" {
		if !strings.HasPrefix(config.HealthPath, "/") || strings.HasPrefix(config.HealthPath, "//") {
			return Health{}, errors.New("health path must start with a single /")
		}
		path, err := url.Parse(config.HealthPath)
		if err != nil || path.Host != "" {
			return Health{}, errors.New("invalid health path")
		}
		u = u.ResolveReference(path)
	}
	u.Fragment = ""
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return Health{}, errors.New("invalid local health URL")
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	start := time.Now()
	response, err := client.Do(request)
	result := Health{URL: u.String(), Duration: time.Since(start).Milliseconds()}
	if err != nil {
		result.Message = "Endpoint unavailable or timed out"
		return result, nil
	}
	defer response.Body.Close()
	result.Status = response.StatusCode
	result.Responding = response.StatusCode >= 200 && response.StatusCode < 300
	result.Message = response.Status
	return result, nil
}
