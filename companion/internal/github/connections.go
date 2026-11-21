package github

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"

	"gitme/companion/internal/credentials"
)

type SavedConnection struct {
	ID     string `json:"id"`
	Legacy bool   `json:"legacy,omitempty"`
	Connection
}

func saved(record connectionRecord, connection Connection) SavedConnection {
	id := record.ID
	if record.Account != nil {
		id = strconv.FormatInt(record.Account.ID, 10)
	}
	return SavedConnection{ID: id, Legacy: record.ID == "legacy", Connection: connection}
}

func matches(record connectionRecord, id string) bool {
	return record.ID == id || (record.Account != nil && strconv.FormatInt(record.Account.ID, 10) == id)
}

type connectionRecord struct {
	ID      string   `json:"id"`
	Account *Account `json:"account,omitempty"`
}

type Connections struct {
	mu         sync.Mutex
	legacy     *Service
	index      credentials.Vault
	factory    func(string) credentials.Vault
	services   map[string]*Service
	records    []connectionRecord
	loaded     bool
	generation uint64
}

func NewConnections(legacy *Service, index credentials.Vault, factory func(string) credentials.Vault) *Connections {
	return &Connections{legacy: legacy, index: index, factory: factory, services: map[string]*Service{"legacy": legacy}}
}

func ValidConnectionID(id string) bool {
	if id == "legacy" {
		return true
	}
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0 && n <= 9007199254740991 && strconv.FormatInt(n, 10) == id
}

func (m *Connections) load() error {
	if m.loaded {
		return nil
	}
	raw, err := m.index.Read()
	if errors.Is(err, credentials.ErrMissing) {
		if _, err := m.legacy.vault.Read(); err == nil {
			m.records = []connectionRecord{{ID: "legacy"}}
		} else if !errors.Is(err, credentials.ErrMissing) {
			return err
		}
	} else if err != nil {
		return err
	} else {
		var records []connectionRecord
		if json.Unmarshal([]byte(raw), &records) != nil || len(records) > 10 {
			return errors.New("GitHub account list could not be read")
		}
		seen := map[string]bool{}
		for _, record := range records {
			if !ValidConnectionID(record.ID) || seen[record.ID] || (record.Account != nil && (record.Account.ID <= 0 || record.Account.ID > 9007199254740991 || !segment.MatchString(record.Account.Login) || (record.ID != "legacy" && record.ID != strconv.FormatInt(record.Account.ID, 10)))) {
				return errors.New("GitHub account list could not be read")
			}
			seen[record.ID] = true
		}
		m.records = records
	}
	m.loaded = true
	return nil
}

func (m *Connections) persist(records []connectionRecord) error {
	raw, err := json.Marshal(records)
	if err != nil {
		return errors.New("GitHub account list could not be saved")
	}
	return m.index.Write(string(raw))
}

func (m *Connections) service(id string) *Service {
	if s := m.services[id]; s != nil {
		return s
	}
	s := New(m.factory(id), m.legacy.client)
	m.services[id] = s
	return s
}

func (m *Connections) Select(id string) (*Service, error) {
	if id == "" {
		id = "legacy"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return nil, err
	}
	for _, record := range m.records {
		if matches(record, id) {
			return m.service(record.ID), nil
		}
	}
	if id == "legacy" {
		return m.legacy, nil
	}
	return nil, errors.New("this GitHub account is disconnected; choose a connected account in the repository picker")
}

func (m *Connections) List(ctx context.Context, refresh bool) ([]SavedConnection, error) {
	m.mu.Lock()
	if err := m.load(); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	records := append([]connectionRecord(nil), m.records...)
	generation := m.generation
	services := make([]*Service, len(records))
	for i, record := range records {
		services[i] = m.service(record.ID)
	}
	m.mu.Unlock()
	results := make([]SavedConnection, len(records))
	var wg sync.WaitGroup
	for i, record := range records {
		wg.Add(1)
		go func(i int, record connectionRecord) {
			defer wg.Done()
			connection, err := services[i].Connection(ctx, refresh)
			if err != nil {
				connection = Connection{Stored: true, Message: err.Error()}
			}
			if connection.Account != nil && record.Account != nil && connection.Account.ID != record.Account.ID {
				connection.Connected = false
				connection.Message = "GitHub account identity changed; replace this account's token"
				connection.Account = record.Account
			}
			if connection.Account == nil {
				connection.Account = record.Account
			}
			results[i] = saved(record, connection)
		}(i, record)
	}
	wg.Wait()
	m.mu.Lock()
	if m.generation != generation {
		m.mu.Unlock()
		return m.List(ctx, false)
	}
	defer m.mu.Unlock()
	output := []SavedConnection{}
	for i, result := range results {
		record := m.records[i]
		if result.Connected && result.Account != nil && record.Account == nil {
			next := append([]connectionRecord(nil), m.records...)
			next[i].Account = &Account{ID: result.Account.ID, Login: result.Account.Login}
			if err := m.persist(next); err != nil {
				return nil, err
			}
			m.records = next
			result = saved(next[i], result.Connection)
		}
		if result.Stored {
			output = append(output, result)
		}
	}
	return output, nil
}

type temporaryToken struct{ value string }

func (v *temporaryToken) Read() (string, error) {
	if v.value == "" {
		return "", credentials.ErrMissing
	}
	return v.value, nil
}
func (v *temporaryToken) Write(value string) error { v.value = value; return nil }
func (v *temporaryToken) Delete() error            { v.value = ""; return nil }

func (m *Connections) Connect(ctx context.Context, token, replaceID string) (SavedConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return SavedConnection{}, err
	}
	if replaceID != "" && !ValidConnectionID(replaceID) {
		return SavedConnection{}, errors.New("choose a saved GitHub account")
	}
	for i, record := range m.records {
		if record.ID == "legacy" && record.Account == nil {
			current, err := m.legacy.Connection(ctx, false)
			if err == nil && current.Connected && current.Account != nil {
				next := append([]connectionRecord(nil), m.records...)
				next[i].Account = &Account{ID: current.Account.ID, Login: current.Account.Login}
				if err := m.persist(next); err != nil {
					return SavedConnection{}, err
				}
				m.records = next
			}
		}
	}
	temp := &temporaryToken{}
	defer temp.Delete()
	candidate := New(temp, m.legacy.client)
	connection, err := candidate.Connect(ctx, token)
	if err != nil {
		return SavedConnection{}, err
	}
	id := strconv.FormatInt(connection.Account.ID, 10)
	position := -1
	for i, record := range m.records {
		if replaceID != "" && matches(record, replaceID) {
			if record.Account == nil || record.Account.ID != connection.Account.ID {
				return SavedConnection{}, errors.New("this token belongs to a different account; use Add account instead")
			}
			id, position = record.ID, i
			break
		}
		if replaceID == "" && record.Account != nil && record.Account.ID == connection.Account.ID {
			id, position = record.ID, i
			break
		}
	}
	if replaceID != "" && position < 0 {
		return SavedConnection{}, errors.New("choose a saved GitHub account")
	}
	if position < 0 && len(m.records) >= 10 {
		return SavedConnection{}, errors.New("up to ten GitHub accounts can be saved on this PC")
	}
	if len(m.records) == 0 {
		id = "legacy"
	}
	next := append([]connectionRecord(nil), m.records...)
	record := connectionRecord{ID: id, Account: &Account{ID: connection.Account.ID, Login: connection.Account.Login}}
	if position < 0 {
		next = append(next, record)
	} else {
		next[position] = record
	}
	s := m.service(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, readErr := s.vault.Read()
	if readErr != nil && !errors.Is(readErr, credentials.ErrMissing) {
		return SavedConnection{}, readErr
	}
	if err := s.vault.Write(temp.value); err != nil {
		return SavedConnection{}, err
	}
	if err := m.persist(next); err != nil {
		var rollbackErr error
		if readErr == nil {
			rollbackErr = s.vault.Write(previous)
		} else {
			rollbackErr = s.vault.Delete()
		}
		if rollbackErr != nil {
			return SavedConnection{}, errors.New("GitHub credential storage failed; check this account's connection before retrying")
		}
		return SavedConnection{}, err
	}
	s.connection = connection
	s.connectionKey = candidate.connectionKey
	s.cache = map[string]cached{}
	m.records = next
	m.generation++
	return saved(record, connection), nil
}

func (m *Connections) Disconnect(id string) error {
	if id == "" {
		id = "legacy"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.load(); err != nil {
		return err
	}
	for i, record := range m.records {
		if !matches(record, id) {
			continue
		}
		if err := m.service(record.ID).Disconnect(); err != nil {
			return err
		}
		m.generation++
		if record.ID == "legacy" {
			return nil
		}
		next := append([]connectionRecord(nil), m.records[:i]...)
		next = append(next, m.records[i+1:]...)
		if err := m.persist(next); err != nil {
			return err
		}
		m.records = next
		delete(m.services, record.ID)
		return nil
	}
	return errors.New("choose a saved GitHub account")
}
