package workload

import (
	"reflect"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/gen"
)

func TestProbeArgsAreDeterministic(t *testing.T) {
	p := Probe{ID: "by_id", SQL: "SELECT $1, $2", Parameters: []gen.Spec{
		{Generator: "int_range", Min: ptrF(1), Max: ptrF(1_000_000)},
		{Generator: "uuid"},
	}}
	a, err := probeArgs(p, "1.0", 42, "repeat-1")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := probeArgs(p, "1.0", 42, "repeat-1")
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same seed and stream must draw the same values: %v vs %v", a, b)
	}
	c, _ := probeArgs(p, "2.0", 42, "repeat-1")
	if reflect.DeepEqual(a, c) {
		t.Fatalf("different versions should draw independent values: %v", a)
	}
}

func ptrF(v float64) *float64 { return &v }
