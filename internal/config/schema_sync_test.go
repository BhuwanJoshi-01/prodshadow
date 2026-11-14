package config

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/schemas"
)

type schemaNode struct {
	Properties map[string]schemaNode `json:"properties"`
}

// TestSchemaMatchesStruct keeps schemas/config.v1.schema.json and the Go
// model in sync: every YAML key must be documented and vice versa.
func TestSchemaMatchesStruct(t *testing.T) {
	raw, err := schemas.Get("config")
	if err != nil {
		t.Fatal(err)
	}
	var root schemaNode
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	compare(t, "", reflect.TypeOf(Config{}), root)
}

func compare(t *testing.T, path string, typ reflect.Type, node schemaNode) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || node.Properties == nil {
		return
	}
	fields := yamlFields(typ)
	var missingInSchema, missingInStruct []string
	for name, ft := range fields {
		child, ok := node.Properties[name]
		if !ok {
			missingInSchema = append(missingInSchema, joinPath(path, name))
			continue
		}
		compare(t, joinPath(path, name), ft, child)
	}
	for name := range node.Properties {
		if _, ok := fields[name]; !ok {
			missingInStruct = append(missingInStruct, joinPath(path, name))
		}
	}
	sort.Strings(missingInSchema)
	sort.Strings(missingInStruct)
	for _, p := range missingInSchema {
		t.Errorf("%s is in the Go model but not in the schema", p)
	}
	for _, p := range missingInStruct {
		t.Errorf("%s is in the schema but not in the Go model", p)
	}
}
