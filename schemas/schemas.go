// Package schemas embeds the JSON Schemas that define ProdShadow's public
// file formats: the configuration, the workload definition and the report.
package schemas

import (
	_ "embed"
	"fmt"
	"sort"
)

var (
	//go:embed report.v1.schema.json
	reportV1 []byte
	//go:embed config.v1.schema.json
	configV1 []byte
	//go:embed workload.v1.schema.json
	workloadV1 []byte
)

var all = map[string][]byte{
	"report":   reportV1,
	"config":   configV1,
	"workload": workloadV1,
}

// Get returns the schema with the given name.
func Get(name string) ([]byte, error) {
	b, ok := all[name]
	if !ok {
		return nil, fmt.Errorf("unknown schema %q (available: %v)", name, Names())
	}
	return b, nil
}

// Names lists available schemas.
func Names() []string {
	out := make([]string, 0, len(all))
	for k := range all {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
