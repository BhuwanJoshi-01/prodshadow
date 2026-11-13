package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestEveryKeyIsDocumented fails when a configuration key is added without
// updating docs/configuration.md.
func TestEveryKeyIsDocumented(t *testing.T) {
	doc, err := os.ReadFile("../../docs/configuration.md")
	if err != nil {
		t.Fatal(err)
	}
	var walkType func(prefix string, typ reflect.Type)
	walkType = func(prefix string, typ reflect.Type) {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		for name, ft := range yamlFields(typ) {
			path := joinPath(prefix, name)
			base := ft
			for base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			if base.Kind() == reflect.Struct {
				walkType(path, base)
				continue
			}
			// Accept any dotted suffix, e.g. `generator.path` for
			// fixture.generator.path.
			parts := strings.Split(path, ".")
			found := false
			for i := range parts {
				if strings.Contains(string(doc), "`"+strings.Join(parts[i:], ".")+"`") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s is not documented in docs/configuration.md", path)
			}
		}
	}
	walkType("", reflect.TypeOf(Config{}))
}
