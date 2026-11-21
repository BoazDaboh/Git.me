package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"gitme/companion/internal/releases"
)

func (s *Store) BeginDeployment(ctx context.Context, action releases.DeploymentAction) error {
	if action.ID == "" || len(action.ID) > 100 || !action.Valid() || action.Environment != "preview" && action.Environment != "production" || action.Result.Outcome != "submitting" {
		return errors.New("invalid deployment action")
	}
	raw, err := json.Marshal(action)
	if err != nil {
		return err
	}
	projectID, serviceID := action.Scope()
	_, err = s.db.ExecContext(ctx, `INSERT INTO deployment_actions(id,project_id,service_id,created_at,payload) VALUES(?,?,?,?,?)`, action.ID, projectID, serviceID, action.CreatedAt.Format("2006-01-02T15:04:05.000000000Z"), string(raw))
	return err
}
func (s *Store) FinishDeployment(ctx context.Context, action releases.DeploymentAction) error {
	if action.Result.Outcome != "accepted" && action.Result.Outcome != "rejected" && action.Result.Outcome != "unconfirmed" && action.Result.Outcome != "not-sent" {
		return errors.New("invalid deployment outcome")
	}
	raw, err := json.Marshal(action)
	if err != nil {
		return err
	}
	projectID, serviceID := action.Scope()
	result, err := s.db.ExecContext(ctx, `UPDATE deployment_actions SET payload=? WHERE id=? AND project_id=? AND service_id=? AND json_extract(payload,'$.result.outcome')='submitting'`, string(raw), action.ID, projectID, serviceID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("deployment action has already finished or is missing")
	}
	return nil
}
func (s *Store) DeploymentActions(ctx context.Context, projectID, serviceID string) ([]releases.DeploymentAction, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM deployment_actions WHERE project_id=? AND service_id=? ORDER BY created_at DESC LIMIT 10`, projectID, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []releases.DeploymentAction{}
	for rows.Next() {
		var raw string
		var action releases.DeploymentAction
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(raw), &action) != nil || !action.Valid() {
			return nil, errors.New("deployment history is unreadable")
		}
		result = append(result, action)
	}
	return result, rows.Err()
}
