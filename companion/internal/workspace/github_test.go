package workspace

import (
	"encoding/json"
	"testing"
)

func TestRepositoryConnectionValidation(t *testing.T) {
	for _, raw := range []string{
		`{"id":42,"fullName":"owner/repo"}`,
		`{"id":42,"fullName":"owner/repo","connectionId":"legacy"}`,
		`{"id":42,"fullName":"owner/repo","connectionId":"123"}`,
	} {
		if err := validateGitHub(json.RawMessage(raw)); err != nil {
			t.Fatal("valid old or account-specific link rejected", err)
		}
	}
	for _, raw := range []string{
		`{"id":42,"fullName":"owner/repo","connectionId":"../legacy"}`,
		`{"id":42,"fullName":"owner/repo","connectionId":"01"}`,
		`{"id":42,"fullName":"owner/repo","connectionId":123}`,
		`{"id":42,"fullName":"owner/repo","connectionId":null}`,
		`{"id":42,"fullName":"owner/repo","token":"secret"}`,
		`{"id":42,"fullName":"owner/repo","connectionId":"9007199254740992"}`,
	} {
		if err := validateGitHub(json.RawMessage(raw)); err == nil {
			t.Fatal("invalid or secret-bearing account link accepted")
		}
	}
}
