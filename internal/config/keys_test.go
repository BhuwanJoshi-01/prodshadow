package config

import (
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func indexFor(t *testing.T, doc string) *keyIndex {
	t.Helper()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(doc), &n); err != nil {
		t.Fatal(err)
	}
	k := newKeyIndex()
	k.walk(&n, reflect.TypeOf(Config{}), "")
	return k
}

func TestUnknownKeyWithSuggestion(t *testing.T) {
	// The misspelling is the point of this test.
	k := indexFor(t, "version: 1\npostgres:\n  verison: \"17\"\n") //nolint:misspell
	if len(k.unknown) != 1 {
		t.Fatalf("expected one unknown key, got %v", k.unknown)
	}
	p := k.unknown[0]
	if p.Path != "postgres.verison" || p.Line != 3 {
		t.Fatalf("unexpected problem %+v", p)
	}
	if !strings.Contains(p.Message, `did you mean "version"`) {
		t.Fatalf("missing suggestion: %s", p.Message)
	}
}

func TestKnownNestedKeysRecordLines(t *testing.T) {
	k := indexFor(t, "postgres:\n  isolation:\n    provider: docker\nmigration:\n  files:\n    - a.sql\n    - b.sql\n")
	if len(k.unknown) != 0 {
		t.Fatalf("unexpected unknown keys: %v", k.unknown)
	}
	if k.lines["postgres.isolation.provider"] != 3 {
		t.Fatalf("provider line = %d", k.lines["postgres.isolation.provider"])
	}
	if k.lines["migration.files[1]"] != 7 {
		t.Fatalf("files[1] line = %d", k.lines["migration.files[1]"])
	}
}

func TestMapKeysAreNotUnknown(t *testing.T) {
	k := indexFor(t, "postgres:\n  settings:\n    shared_buffers: 128MB\npolicy:\n  rules:\n    PS101: off\n")
	if len(k.unknown) != 0 {
		t.Fatalf("map keys should be free-form: %v", k.unknown)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{{"", "", 0}, {"abc", "abc", 0}, {"abc", "abd", 1}, {"kitten", "sitting", 3}, {"", "ab", 2}}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
