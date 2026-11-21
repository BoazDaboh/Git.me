package workspace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFocusElapsedDurationValidation(t *testing.T) {
	for _, value := range []string{"0", "1500", "9007199254740991"} {
		raw := strings.Replace(sample, `"started":300`, `"started":300,"elapsedMs":`+value, 1)
		if _, err := Validate(json.RawMessage(raw)); err != nil {
			t.Fatalf("rejected %s: %v", value, err)
		}
	}
	for _, value := range []string{"null", `"1500"`, "-1", "1.5", "9007199254740992"} {
		raw := strings.Replace(sample, `"started":300`, `"started":300,"elapsedMs":`+value, 1)
		if _, err := Validate(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}
