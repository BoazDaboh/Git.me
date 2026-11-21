package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"gitme/companion/internal/releases"
	"time"
)

func (s *Store) RecordRelease(ctx context.Context, record releases.Record) (releases.Record, error) {
	if !record.Valid() {
		return releases.Record{}, errors.New("invalid release record")
	}
	record.ID = record.Key()
	record.CreatedAt = time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return record, err
	}
	defer tx.Rollback()
	var old string
	readErr := tx.QueryRowContext(ctx, "SELECT payload FROM release_records WHERE id=?", record.ID).Scan(&old)
	if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
		return record, readErr
	}
	if readErr == nil {
		var previous releases.Record
		if json.Unmarshal([]byte(old), &previous) != nil {
			return record, errors.New("release record is unreadable")
		}
		record.CreatedAt = previous.CreatedAt
		if previous.Source == "push" {
			record.Source = "push"
		}
		record.RemoteVerified = record.RemoteVerified || previous.RemoteVerified
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return record, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO release_records(id,project_id,service_id,created_at,payload) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload`, record.ID, record.ProjectID, record.ServiceID, record.CreatedAt.Format("2006-01-02T15:04:05.000000000Z"), string(payload))
	if err != nil {
		return record, err
	}
	return record, tx.Commit()
}
func (s *Store) Releases(ctx context.Context, projectID, serviceID string) ([]releases.Record, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT payload FROM release_records WHERE project_id=? AND service_id=? ORDER BY created_at DESC LIMIT 10", projectID, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []releases.Record{}
	for rows.Next() {
		var payload string
		var record releases.Record
		if err = rows.Scan(&payload); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(payload), &record) != nil || !record.Valid() {
			return nil, errors.New("release history is unreadable")
		}
		result = append(result, record)
	}
	return result, rows.Err()
}
