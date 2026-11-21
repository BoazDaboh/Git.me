package apitester

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const MaxBody = 1 << 20
const MaxJSON = 64 << 10

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Input struct {
	URL        string   `json:"url"`
	Headers    []Header `json:"headers"`
	Method     string   `json:"method"`
	Body       string   `json:"body"`
	StatusOnly bool     `json:"-"`
}
type Result struct {
	Status    int      `json:"status"`
	Duration  int64    `json:"duration_ms"`
	Bytes     int      `json:"bytes"`
	Body      string   `json:"body"`
	Headers   []Header `json:"headers"`
	Truncated bool     `json:"truncated"`
	Binary    bool     `json:"binary"`
	Message   string   `json:"message"`
	Delivery  string   `json:"delivery"`
}

var headerName = regexp.MustCompile(`^[A-Za-z0-9-]{1,80}$`)
var deniedHeaders = map[string]bool{"host": true, "connection": true, "content-length": true, "transfer-encoding": true, "upgrade": true, "te": true, "trailer": true, "keep-alive": true, "proxy-authorization": true, "proxy-connection": true, "accept-encoding": true, "x-gitme-client": true, "forwarded": true, "x-forwarded-for": true, "x-forwarded-host": true, "x-forwarded-proto": true}

func URL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || strings.ContainsAny(raw, "\x00\r\n\t") || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Host == "" || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || strings.Contains(u.Hostname(), "%") {
		return nil, errors.New("enter an HTTP or HTTPS URL without embedded credentials or a fragment")
	}
	port := u.Port()
	if port != "" {
		p, e := strconv.Atoi(port)
		if e != nil || p < 1 || p > 65535 {
			return nil, errors.New("invalid URL port")
		}
		if p == 3015 || p == 47821 {
			return nil, errors.New("Git/Me's own app and companion ports cannot be tested")
		}
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "metadata.google.internal" || host == "metadata.google" {
		return nil, errors.New("cloud metadata addresses cannot be tested")
	}
	if ip, err := netip.ParseAddr(host); err == nil && !allowedIP(ip) {
		return nil, errors.New("metadata, link-local, multicast and unspecified addresses cannot be tested")
	}
	return u, nil
}
func allowedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() {
		return true
	}
	if !ip.IsGlobalUnicast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	for _, raw := range []string{"100.64.0.0/10", "0.0.0.0/8", "198.18.0.0/15", "192.0.0.0/24", "240.0.0.0/4", "fd00:ec2::254/128", "168.63.129.16/32"} {
		if netip.MustParsePrefix(raw).Contains(ip) {
			return false
		}
	}
	return true
}

func validate(input Input) error {
	method := input.Method
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodPost {
		return errors.New("only GET and reviewed JSON POST are supported")
	}
	if input.StatusOnly && method != http.MethodGet {
		return errors.New("saved status checks support GET only")
	}
	if method == http.MethodGet && input.Body != "" {
		return errors.New("GET requests cannot contain a body")
	}
	if method == http.MethodPost && (len(input.Body) > MaxJSON || !utf8.ValidString(input.Body) || !json.Valid([]byte(input.Body))) {
		return errors.New("enter valid JSON, up to 64 KiB")
	}
	if _, err := URL(input.URL); err != nil {
		return err
	}
	if len(input.Headers) > 24 {
		return errors.New("use at most 24 request headers")
	}
	size := 0
	seen := map[string]bool{}
	for _, h := range input.Headers {
		name := strings.ToLower(h.Name)
		size += len(h.Name) + len(h.Value)
		if !headerName.MatchString(h.Name) || deniedHeaders[name] || strings.ContainsAny(h.Value, "\x00\r\n") || len(h.Value) > 4096 || size > 16<<10 || seen[name] {
			return errors.New("invalid, duplicate or reserved request header")
		}
		seen[name] = true
		if method == http.MethodPost && name == "content-type" {
			kind, params, err := mime.ParseMediaType(h.Value)
			if err != nil || kind != "application/json" || params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
				return errors.New("POST uses application/json with UTF-8 encoding")
			}
		}
	}
	return nil
}

type Runner struct {
	Resolve func(context.Context, string) ([]netip.Addr, error)
	Dial    func(context.Context, string, string) (net.Conn, error)
}

func (runner Runner) Run(ctx context.Context, input Input) (Result, error) {
	if err := validate(input); err != nil {
		return Result{}, err
	}
	u, _ := URL(input.URL)
	resolve := runner.Resolve
	if resolve == nil {
		resolve = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	dial := runner.Dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: 4 * time.Second}).DialContext
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, MaxResponseHeaderBytes: 32 << 10, TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 8 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("invalid destination")
		}
		if !strings.EqualFold(host, u.Hostname()) {
			return nil, errors.New("destination changed")
		}
		ips, err := resolve(ctx, host)
		if err != nil || len(ips) == 0 || len(ips) > 32 {
			return nil, errors.New("destination could not be resolved")
		}
		for _, ip := range ips {
			if !allowedIP(ip) {
				return nil, errors.New("destination resolves to a blocked address")
			}
		}
		// Dial the validated address directly so a second DNS answer cannot change the destination.
		var last error
		for _, ip := range ips {
			connection, e := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if e == nil {
				return connection, nil
			}
			last = e
			if ctx.Err() != nil {
				break
			}
		}
		return nil, last
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	method := input.Method
	if method == "" {
		method = http.MethodGet
	}
	var requestBody io.Reader
	if method == http.MethodPost {
		requestBody = strings.NewReader(input.Body)
	}
	var wrote atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{WroteHeaders: func() { wrote.Store(true) }})
	req, err := http.NewRequestWithContext(ctx, method, u.String(), requestBody)
	if err != nil {
		return Result{}, errors.New("could not prepare request")
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "GitMe-API-tester/1")
	req.GetBody = nil
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, h := range input.Headers {
		req.Header.Set(h.Name, h.Value)
	}
	start := time.Now()
	response, err := client.Do(req)
	result := Result{Duration: time.Since(start).Milliseconds(), Headers: []Header{}, Delivery: "not-sent"}
	if err != nil {
		result.Message = "Endpoint unavailable. Check the address, server, network and HTTPS certificate."
		if ctx.Err() != nil {
			result.Message = "Request stopped or timed out after 10 seconds."
		}
		if method == http.MethodPost && wrote.Load() {
			result.Delivery = "unconfirmed"
			result.Message = "The backend may have processed this POST, but its response was unavailable. Check the target before sending again. Git/Me will not retry it."
		}
		return result, nil
	}
	defer response.Body.Close()
	result.Status = response.StatusCode
	result.Delivery = "response"
	if input.StatusOnly {
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			result.Message = "Redirect returned without following it."
		}
		return result, nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxBody+1))
	result.Duration = time.Since(start).Milliseconds()
	if err != nil {
		result.Message = "Response interrupted before it was fully read."
	}
	if len(raw) > MaxBody {
		raw = raw[:MaxBody]
		result.Truncated = true
	}
	result.Bytes = len(raw)
	typeName := strings.ToLower(response.Header.Get("Content-Type"))
	result.Binary = !utf8.Valid(raw) || strings.IndexByte(string(raw), 0) >= 0 || typeName != "" && !strings.HasPrefix(typeName, "text/") && !strings.Contains(typeName, "json") && !strings.Contains(typeName, "xml") && !strings.Contains(typeName, "javascript") && !strings.Contains(typeName, "x-www-form-urlencoded")
	if !result.Binary {
		result.Body = string(raw)
	}
	for name, values := range response.Header {
		for _, value := range values {
			lower := strings.ToLower(name)
			if lower == "set-cookie" || lower == "authorization" || lower == "proxy-authenticate" || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "api-key") {
				value = "[hidden]"
			}
			result.Headers = append(result.Headers, Header{name, value})
		}
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		result.Message = "Redirect returned. Git/Me did not follow it."
	}
	return result, nil
}
