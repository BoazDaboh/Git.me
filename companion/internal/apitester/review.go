package apitester

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type PostPlan struct {
	Token       string    `json:"token"`
	URL         string    `json:"url"`
	BodyBytes   int       `json:"body_bytes"`
	HeaderNames []string  `json:"header_names"`
	ExpiresAt   time.Time `json:"expires_at"`
}
type postRecord struct {
	scope, fingerprint string
	input              Input
	plan               PostPlan
}
type Reviews struct {
	mu    sync.Mutex
	plans map[string]postRecord
	now   func() time.Time
}

func NewReviews() *Reviews { return &Reviews{plans: map[string]postRecord{}, now: time.Now} }
func (m *Reviews) Prepare(scope, fingerprint string, input Input) (PostPlan, error) {
	if input.Method != "POST" {
		return PostPlan{}, errors.New("choose POST to review JSON data")
	}
	if err := validate(input); err != nil {
		return PostPlan{}, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return PostPlan{}, errors.New("could not create request review")
	}
	plan := PostPlan{Token: hex.EncodeToString(key), URL: input.URL, BodyBytes: len(input.Body), HeaderNames: []string{}, ExpiresAt: m.now().UTC().Add(5 * time.Minute)}
	for _, header := range input.Headers {
		plan.HeaderNames = append(plan.HeaderNames, header.Name)
	}
	input.Headers = append([]Header(nil), input.Headers...)
	m.mu.Lock()
	defer m.mu.Unlock()
	for token, p := range m.plans {
		if m.now().After(p.plan.ExpiresAt) || p.scope == scope {
			delete(m.plans, token)
		}
	}
	if len(m.plans) >= 64 {
		return PostPlan{}, errors.New("too many active request reviews; close an unused review")
	}
	m.plans[plan.Token] = postRecord{scope, fingerprint, input, plan}
	return plan, nil
}
func (m *Reviews) Execute(ctx context.Context, scope, fingerprint, token string, runner Runner) (Result, error) {
	m.mu.Lock()
	record, exists := m.plans[token]
	if !exists || record.scope != scope {
		m.mu.Unlock()
		return Result{}, errors.New("POST review is missing, used or belongs to another service. Review again; no request was sent")
	}
	delete(m.plans, token)
	m.mu.Unlock()
	if m.now().After(record.plan.ExpiresAt) || record.fingerprint != fingerprint {
		return Result{}, errors.New("POST review expired or service settings changed. Review again; no request was sent")
	}
	return runner.Run(ctx, record.input)
}
func (m *Reviews) Cancel(scope, token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.plans[token]; ok && p.scope == scope {
		delete(m.plans, token)
	}
}
