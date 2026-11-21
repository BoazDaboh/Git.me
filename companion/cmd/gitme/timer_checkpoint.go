package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gitme/companion/internal/workspace"
)

const timerCheckpointInterval = 5 * time.Second

var timerCheckpointMu sync.Mutex

type timerCheckpoint struct {
	DatabaseID string `json:"database_id"`
	Started    int64  `json:"started"`
	Checked    int64  `json:"checked"`
}
type activeTimer struct {
	Project   string `json:"project"`
	Started   int64  `json:"started"`
	ElapsedMS int64  `json:"elapsedMs,omitempty"`
}
type timerSample struct{ Wall, Awake int64 }

func checkpointPath(store *workspace.Store) string {
	return filepath.Join(filepath.Dir(store.Path), "timer-checkpoint.json")
}
func currentTimer(raw json.RawMessage) (map[string]json.RawMessage, *activeTimer, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, nil, err
	}
	if string(data["focus"]) == "null" {
		return data, nil, nil
	}
	var focus activeTimer
	if err := json.Unmarshal(data["focus"], &focus); err != nil || focus.Started <= 0 || focus.ElapsedMS < 0 {
		return nil, nil, errors.New("invalid timer")
	}
	return data, &focus, nil
}
func readTimerCheckpoint(store *workspace.Store) (timerCheckpoint, error) {
	raw, err := os.ReadFile(checkpointPath(store))
	if err != nil {
		return timerCheckpoint{}, err
	}
	var checkpoint timerCheckpoint
	if len(raw) > 4096 || json.Unmarshal(raw, &checkpoint) != nil || checkpoint.DatabaseID == "" || checkpoint.Started <= 0 || checkpoint.Checked < checkpoint.Started {
		return checkpoint, errors.New("invalid timer checkpoint")
	}
	return checkpoint, nil
}
func saveTimerCheckpoint(store *workspace.Store, checkpoint timerCheckpoint) error {
	content, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	path := checkpointPath(store)
	file, err := os.CreateTemp(filepath.Dir(path), ".timer-*.tmp")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func writeTimerCheckpointLocked(ctx context.Context, store *workspace.Store, now int64) error {
	snapshot, err := store.Load(ctx)
	if err != nil || snapshot.Revision == 0 {
		return err
	}
	_, focus, err := currentTimer(snapshot.Workspace)
	if err != nil {
		return err
	}
	if focus == nil {
		err = os.Remove(checkpointPath(store))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if now < focus.Started {
		return errors.New("timer starts after the current clock")
	}
	return saveTimerCheckpoint(store, timerCheckpoint{snapshot.DatabaseID, focus.Started, now})
}
func writeTimerCheckpoint(ctx context.Context, store *workspace.Store, now int64) error {
	timerCheckpointMu.Lock()
	defer timerCheckpointMu.Unlock()
	return writeTimerCheckpointLocked(ctx, store, now)
}
func ensureTimerCheckpoint(ctx context.Context, store *workspace.Store, databaseID string, raw json.RawMessage) error {
	timerCheckpointMu.Lock()
	defer timerCheckpointMu.Unlock()
	_, focus, err := currentTimer(raw)
	if err != nil || focus == nil {
		return err
	}
	existing, err := readTimerCheckpoint(store)
	if err == nil && existing.DatabaseID == databaseID && existing.Started == focus.Started {
		return nil
	}
	return writeTimerCheckpointLocked(ctx, store, time.Now().UnixMilli())
}
func stopInterruptedTimer(ctx context.Context, store *workspace.Store, checkpoint timerCheckpoint, ended, resumed int64) (bool, error) {
	snapshot, err := store.Load(ctx)
	if err != nil || snapshot.Revision == 0 || snapshot.DatabaseID != checkpoint.DatabaseID {
		return false, err
	}
	data, focus, err := currentTimer(snapshot.Workspace)
	if err != nil || focus == nil || focus.Started != checkpoint.Started {
		return false, err
	}
	if ended < focus.Started || ended > resumed || resumed <= focus.Started {
		return false, errors.New("invalid timer recovery interval")
	}
	var sessions []json.RawMessage
	if err = json.Unmarshal(data["sessions"], &sessions); err != nil {
		return false, err
	}
	if ended > focus.Started {
		entry, marshalErr := json.Marshal(struct {
			Project string `json:"project"`
			Started int64  `json:"started"`
			Ended   int64  `json:"ended"`
		}{focus.Project, focus.Started, ended})
		if marshalErr != nil {
			return false, marshalErr
		}
		sessions = append(sessions, entry)
	}
	data["sessions"], err = json.Marshal(sessions)
	if err != nil {
		return false, err
	}
	data["focus"] = json.RawMessage("null")
	merged, err := json.Marshal(data)
	if err != nil {
		return false, err
	}
	if _, err = store.Save(ctx, snapshot.DatabaseID, snapshot.Revision, merged); err != nil {
		return false, err
	}
	if err = os.Remove(checkpointPath(store)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return true, err
	}
	return true, nil
}
func recoverTimerCheckpoint(ctx context.Context, store *workspace.Store, now int64) (bool, error) {
	timerCheckpointMu.Lock()
	defer timerCheckpointMu.Unlock()
	snapshot, err := store.Load(ctx)
	if err != nil || snapshot.Revision == 0 {
		return false, err
	}
	_, focus, err := currentTimer(snapshot.Workspace)
	if err != nil || focus == nil {
		return false, err
	}
	if focus.Started == now {
		return false, nil
	}
	checkpoint, err := readTimerCheckpoint(store)
	if err != nil || checkpoint.DatabaseID != snapshot.DatabaseID || checkpoint.Started != focus.Started || checkpoint.Checked > now {
		checkpoint = timerCheckpoint{snapshot.DatabaseID, focus.Started, focus.Started}
	}
	return stopInterruptedTimer(ctx, store, checkpoint, checkpoint.Checked, now)
}
func timerInterrupted(previous, current timerSample) bool {
	wall := current.Wall - previous.Wall
	if previous.Wall == 0 {
		return false
	}
	if wall > (3 * timerCheckpointInterval).Milliseconds() {
		return true
	}
	return previous.Awake > 0 && current.Awake >= previous.Awake && wall-(current.Awake-previous.Awake) > 2000
}
func checkpointTimerTick(ctx context.Context, store *workspace.Store, previous, current timerSample) error {
	timerCheckpointMu.Lock()
	defer timerCheckpointMu.Unlock()
	sleep, wake := systemSleepInterval()
	checkpoint, err := readTimerCheckpoint(store)
	if err == nil && (timerInterrupted(previous, current) || (sleep >= checkpoint.Started && wake >= sleep && wake <= current.Wall)) {
		ended := checkpoint.Checked
		if sleep >= checkpoint.Checked && sleep <= current.Wall && wake >= sleep {
			ended = sleep
		}
		if _, err = stopInterruptedTimer(ctx, store, checkpoint, ended, current.Wall); err != nil {
			return err
		}
	}
	return writeTimerCheckpointLocked(ctx, store, current.Wall)
}
func runTimerCheckpoints(ctx context.Context, store *workspace.Store) {
	ticker := time.NewTicker(timerCheckpointInterval)
	defer ticker.Stop()
	previous := timerSample{time.Now().UnixMilli(), systemAwakeMillis()}
	for {
		select {
		case <-ctx.Done():
			final, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := checkpointTimerTick(final, store, previous, timerSample{time.Now().UnixMilli(), systemAwakeMillis()}); err != nil {
				log.Printf("Final timer checkpoint: %v", err)
			}
			cancel()
			return
		case <-ticker.C:
			current := timerSample{time.Now().UnixMilli(), systemAwakeMillis()}
			if err := checkpointTimerTick(ctx, store, previous, current); err != nil && !errors.Is(err, workspace.ErrConflict) {
				log.Printf("Timer checkpoint: %v", err)
			}
			previous = current
		}
	}
}
