package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/schemas"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	raw, err := schemas.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	url := name + ".json"
	if err := c.AddResource(url, doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile(url)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestReportsValidateAgainstSchema(t *testing.T) {
	s := compileSchema(t, "report")
	in := sampleInput(t)
	variants := map[string]*Report{"block": Build(in)}
	in.Errors = []error{errors.New("boom")}
	variants["failed"] = Build(in)
	for name, r := range variants {
		js, err := r.JSON()
		if err != nil {
			t.Fatal(err)
		}
		v, err := jsonschema.UnmarshalJSON(bytes.NewReader(js))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Validate(v); err != nil {
			t.Errorf("%s report does not match schema: %v", name, err)
		}
	}
}

func TestSchemaRejectsUnknownStatus(t *testing.T) {
	s := compileSchema(t, "report")
	js, _ := Build(sampleInput(t)).JSON()
	var m map[string]any
	if err := json.Unmarshal(js, &m); err != nil {
		t.Fatal(err)
	}
	m["status"] = "safe_for_production"
	if err := s.Validate(m); err == nil {
		t.Fatal("schema must reject statuses that claim safety")
	}
}
