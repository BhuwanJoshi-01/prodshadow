package schemas

import (
	"encoding/json"
	"testing"
)

func TestEverySchemaIsValidJSON(t *testing.T) {
	for _, name := range Names() {
		raw, err := Get(name)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if doc["$schema"] == nil || doc["$id"] == nil || doc["title"] == nil {
			t.Errorf("%s: missing $schema, $id or title", name)
		}
	}
	if _, err := Get("nope"); err == nil {
		t.Fatal("unknown schema should fail")
	}
}

func TestConfigSchemaHasNoConnectionStringField(t *testing.T) {
	raw, _ := Get("config")
	var doc struct {
		Properties map[string]struct {
			Properties map[string]any `json:"properties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for section, s := range doc.Properties {
		for key := range s.Properties {
			for _, banned := range []string{"dsn", "url", "host", "connection_string", "password"} {
				if key == banned {
					t.Errorf("%s.%s: configuration must not accept a database address or credential", section, key)
				}
			}
		}
	}
}
