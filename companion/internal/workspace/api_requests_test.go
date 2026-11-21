package workspace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAPIRequestTemplatesPersistWithoutCredentials(t *testing.T) {
	valid := `[{"id":"req_one","name":"Communities","method":"GET","url":"http://localhost:8080/communities","serviceId":"s1","updatedAt":100}]`
	if err := validateAPIRequests(json.RawMessage(valid)); err != nil {
		t.Fatal(err)
	}
	if err := validateAPIRequests(json.RawMessage(strings.Replace(valid, `"GET"`, `"POST"`, 1))); err != nil {
		t.Fatal(err)
	}
	withExpected := strings.Replace(valid, `"updatedAt":100`, `"expectedStatus":404,"updatedAt":100`, 1)
	if err := validateAPIRequests(json.RawMessage(withExpected)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"199", "600", "200.5", `"200"`, "null"} {
		if err := validateAPIRequests(json.RawMessage(strings.Replace(withExpected, `"expectedStatus":404`, `"expectedStatus":`+bad, 1))); err == nil {
			t.Fatal("invalid status accepted", bad)
		}
	}
	for _, bad := range []string{strings.Replace(valid, `"updatedAt":100`, `"body":"private","updatedAt":100`, 1), strings.Replace(valid, `"GET"`, `"DELETE"`, 1), strings.Replace(valid, `/communities"`, `/communities?token=private"`, 1), strings.Replace(valid, `"updatedAt":100`, `"headers":{"Authorization":"private"},"updatedAt":100`, 1), strings.Replace(valid, `"updatedAt":100`, `"response":"private","updatedAt":100`, 1), `null`} {
		if err := validateAPIRequests(json.RawMessage(bad)); err == nil {
			t.Fatal("unsafe template accepted")
		}
	}
	var data map[string]any
	json.Unmarshal([]byte(sample), &data)
	project := data["projects"].([]any)[0].(map[string]any)
	var templates any
	json.Unmarshal([]byte(withExpected), &templates)
	project["apiRequests"] = templates
	raw, _ := json.Marshal(data)
	store := openTest(t)
	before, _ := store.Load(t.Context())
	if _, err := store.Save(t.Context(), before.DatabaseID, before.Revision, raw); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Load(t.Context())
	equalJSON(t, after.Workspace, raw)
	var restored map[string]any
	json.Unmarshal(after.Workspace, &restored)
	delete(restored["projects"].([]any)[0].(map[string]any), "apiRequests")
	base, _ := json.Marshal(restored)
	equalJSON(t, base, []byte(sample))
}
