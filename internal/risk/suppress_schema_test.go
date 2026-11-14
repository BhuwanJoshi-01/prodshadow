package risk

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/schemas"
)

// TestSuppressionSchemaMatchesStruct keeps the published JSON Schema and
// the loader in sync: every YAML key the loader accepts is in the schema
// and the other way round.
func TestSuppressionSchemaMatchesStruct(t *testing.T) {
	raw, err := schemas.Get("suppressions")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties struct {
			Suppressions struct {
				Items struct {
					Properties map[string]any `json:"properties"`
					Required   []string       `json:"required"`
				} `json:"items"`
			} `json:"suppressions"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	st := reflect.TypeFor[Suppression]()
	inSchema := make([]string, 0, len(doc.Properties.Suppressions.Items.Properties))
	inStruct := make([]string, 0, st.NumField())
	for k := range doc.Properties.Suppressions.Items.Properties {
		inSchema = append(inSchema, k)
	}
	for i := range st.NumField() {
		name, _, _ := strings.Cut(st.Field(i).Tag.Get("yaml"), ",")
		inStruct = append(inStruct, name)
	}
	sort.Strings(inSchema)
	sort.Strings(inStruct)
	if !reflect.DeepEqual(inSchema, inStruct) {
		t.Fatalf("schema keys %v, struct keys %v", inSchema, inStruct)
	}
	if got := doc.Properties.Suppressions.Items.Required; !reflect.DeepEqual(got, []string{"rule", "reason"}) {
		t.Fatalf("required = %v", got)
	}
}
