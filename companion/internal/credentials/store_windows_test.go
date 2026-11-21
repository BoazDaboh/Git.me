package credentials

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
)

func TestWindowsCredentialRoundTrip(t *testing.T) {
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	store := &Store{Target: "GitMe/QA/" + hex.EncodeToString(id)}
	t.Cleanup(func() { _ = store.Delete() })
	if _, err := store.Read(); !errors.Is(err, ErrMissing) {
		t.Fatal("fresh credential should not exist")
	}
	if err := store.Write("github_pat_fixture_readonly_1234567890"); err != nil {
		t.Fatal(err)
	}
	reopened := &Store{Target: store.Target}
	read, err := reopened.Read()
	if err != nil || read != "github_pat_fixture_readonly_1234567890" {
		t.Fatal("credential round-trip failed")
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(); !errors.Is(err, ErrMissing) {
		t.Fatal("credential was not deleted")
	}
}
