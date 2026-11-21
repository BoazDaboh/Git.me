package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitme/companion/internal/workspace"
)

func timerTestStore(t *testing.T) *workspace.Store {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "internal", "workspace", "store_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(fixture), "const sample = `", 2)
	if len(parts) != 2 {
		t.Fatal("fixture missing")
	}
	raw := strings.SplitN(parts[1], "`", 2)[0]
	store, err := workspace.Open(filepath.Join(t.TempDir(), "gitme.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	snapshot, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Save(context.Background(), snapshot.DatabaseID, 0, json.RawMessage(raw)); err != nil {
		t.Fatal(err)
	}
	return store
}
func TestTimerRecoveryStopsAndDoesNotDuplicateSession(t *testing.T) {
	ctx := context.Background()
	store := timerTestStore(t)
	if err := writeTimerCheckpoint(ctx, store, 1500); err != nil {
		t.Fatal(err)
	}
	if ok, err := recoverTimerCheckpoint(ctx, store, 10000); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, err := recoverTimerCheckpoint(ctx, store, 20000); ok || err != nil {
		t.Fatal(ok, err)
	}
	snapshot, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, focus, err := currentTimer(snapshot.Workspace)
	if err != nil || focus != nil {
		t.Fatalf("focus: %+v %v", focus, err)
	}
	var data struct {
		Sessions []struct{ Started, Ended int64 }
	}
	if err := json.Unmarshal(snapshot.Workspace, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Sessions) != 2 || data.Sessions[1].Started != 300 || data.Sessions[1].Ended != 1500 {
		t.Fatalf("unexpected history: %+v", data.Sessions)
	}
}
func TestTimerGapWithoutSleepNotificationExcludesDowntime(t *testing.T) {
	ctx := context.Background()
	store := timerTestStore(t)
	before, _ := store.Load(ctx)
	if err := writeTimerCheckpoint(ctx, store, 1500); err != nil {
		t.Fatal(err)
	}
	if err := checkpointTimerTick(ctx, store, timerSample{1500, 1000}, timerSample{100000, 1100}); err != nil {
		t.Fatal(err)
	}
	after, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, focus, err := currentTimer(after.Workspace)
	if err != nil || focus != nil {
		t.Fatalf("counted downtime: %+v %v", focus, err)
	}
	var oldData, newData map[string]json.RawMessage
	json.Unmarshal(before.Workspace, &oldData)
	json.Unmarshal(after.Workspace, &newData)
	delete(oldData, "focus")
	delete(newData, "focus")
	delete(oldData, "sessions")
	delete(newData, "sessions")
	if !reflect.DeepEqual(oldData, newData) {
		t.Fatal("recovery changed unrelated records")
	}
	if err := checkpointTimerTick(ctx, store, timerSample{100000, 1100}, timerSample{105000, 6100}); err != nil {
		t.Fatal(err)
	}
	again, _ := store.Load(ctx)
	if again.Revision != after.Revision {
		t.Fatal("ordinary ticks changed the workspace")
	}
}
func TestTimerInterruptDetection(t *testing.T) {
	for _, test := range []struct {
		name              string
		previous, current timerSample
		want              bool
	}{
		{"normal", timerSample{1000, 1000}, timerSample{6000, 6000}, false},
		{"short sleep", timerSample{1000, 1000}, timerSample{11000, 3000}, true},
		{"missed notification", timerSample{1000, 0}, timerSample{60000, 0}, true},
		{"initial", timerSample{}, timerSample{60000, 1000}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := timerInterrupted(test.previous, test.current); got != test.want {
				t.Fatalf("got %v want %v", got, test.want)
			}
		})
	}
}
func TestTimerShutdownWritesFinalCheckpoint(t *testing.T) {
	store := timerTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runTimerCheckpoints(ctx, store)
	checkpoint, err := readTimerCheckpoint(store)
	if err != nil || checkpoint.Checked <= 300 {
		t.Fatalf("no final checkpoint: %+v %v", checkpoint, err)
	}
}
func TestTimerRecoveryDoesNotRestoreAStoppedOrDifferentSession(t *testing.T) {
	ctx := context.Background()
	for _, focus := range []string{`null`, `{"project":"p1","started":2000}`} {
		store := timerTestStore(t)
		if err := writeTimerCheckpoint(ctx, store, 1500); err != nil {
			t.Fatal(err)
		}
		snapshot, _ := store.Load(ctx)
		var data map[string]json.RawMessage
		json.Unmarshal(snapshot.Workspace, &data)
		data["focus"] = json.RawMessage(focus)
		raw, _ := json.Marshal(data)
		if _, err := store.Save(ctx, snapshot.DatabaseID, snapshot.Revision, raw); err != nil {
			t.Fatal(err)
		}
		ok, err := recoverTimerCheckpoint(ctx, store, 10000)
		if err != nil {
			t.Fatal(err)
		}
		if focus == `null` && ok {
			t.Fatal("restored a stopped session")
		}
		if focus != `null` {
			latest, _ := store.Load(ctx)
			_, timer, err := currentTimer(latest.Workspace)
			if !ok || err != nil || timer != nil {
				t.Fatal("used a different session's checkpoint", timer, err)
			}
		}
	}
}

func TestTimerRecoveryWithoutUsableCheckpointNeverCountsOfflineTime(t *testing.T) {
	for _, content := range []string{"", "invalid JSON", `{"database_id":"another-database","started":300,"checked":1500}`} {
		store := timerTestStore(t)
		if content != "" {
			if err := os.WriteFile(checkpointPath(store), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}
		ok, err := recoverTimerCheckpoint(context.Background(), store, 10000)
		if !ok || err != nil {
			t.Fatal(ok, err)
		}
		snapshot, _ := store.Load(context.Background())
		_, focus, err := currentTimer(snapshot.Workspace)
		if err != nil || focus != nil {
			t.Fatal("counted an unverified interval", focus, err)
		}
	}
}
