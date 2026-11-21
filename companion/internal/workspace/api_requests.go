package workspace

import (
	"encoding/json"
	"errors"
	"regexp"
	"unicode/utf8"

	"gitme/companion/internal/apitester"
)

func validateAPIRequests(raw json.RawMessage) error {
	var requests []map[string]json.RawMessage
	if json.Unmarshal(raw, &requests) != nil || requests == nil || len(requests) > 100 {
		return errors.New("invalid saved API requests")
	}
	seen := map[string]bool{}
	for _, r := range requests {
		_, hasExpected := r["expectedStatus"]
		if len(r) != 6 && !(len(r) == 7 && hasExpected) {
			return errors.New("saved API requests contain only a name, method, URL path, service and timestamp")
		}
		if hasExpected {
			var expected int
			if json.Unmarshal(r["expectedStatus"], &expected) != nil || expected < 200 || expected > 599 {
				return errors.New("expected HTTP status must be a whole number from 200 to 599")
			}
		}
		fields := map[string]string{}
		for _, k := range []string{"id", "name", "method", "url", "serviceId"} {
			var value *string
			if json.Unmarshal(r[k], &value) != nil || value == nil {
				return errors.New("invalid API request field")
			}
			fields[k] = *value
		}
		var updated float64
		u, err := apitester.URL(fields["url"])
		if err != nil || u.RawQuery != "" || u.ForceQuery || (fields["method"] != "GET" && fields["method"] != "POST") || len(fields["name"]) == 0 || utf8.RuneCountInString(fields["name"]) > 80 || len(fields["serviceId"]) > 200 || !regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`).MatchString(fields["id"]) || seen[fields["id"]] || json.Unmarshal(r["updatedAt"], &updated) != nil || updated <= 0 {
			return errors.New("invalid saved API request; query and header values must stay in session")
		}
		seen[fields["id"]] = true
	}
	return nil
}
