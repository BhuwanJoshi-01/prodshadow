package gen

import (
	"math"
	"regexp"
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }
func i(v int64) *int64     { return &v }

func must(t *testing.T, s Spec) Generator {
	t.Helper()
	g, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestStreamsAreDeterministicAndIndependent(t *testing.T) {
	a, b := Stream(42, "orders.customer_id"), Stream(42, "orders.customer_id")
	c := Stream(42, "orders.status")
	same, diff := true, false
	for n := 0; n < 100; n++ {
		x, y, z := a.Uint64(), b.Uint64(), c.Uint64()
		same = same && x == y
		diff = diff || x != z
	}
	if !same || !diff {
		t.Fatalf("same=%v diff=%v", same, diff)
	}
}

func TestIntRangeBounds(t *testing.T) {
	g := must(t, Spec{Generator: "int_range", Min: f(5), Max: f(7)})
	r := Stream(1, "x")
	seen := map[int64]bool{}
	for n := 0; n < 1000; n++ {
		v := g.Next(r).(int64)
		if v < 5 || v > 7 {
			t.Fatalf("out of range: %d", v)
		}
		seen[v] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected all values, saw %v", seen)
	}
}

func TestZipfIsSkewed(t *testing.T) {
	g := must(t, Spec{Generator: "zipf", Min: f(1), Max: f(1000), S: 1.2})
	r := Stream(1, "z")
	counts := map[int64]int{}
	for n := 0; n < 20000; n++ {
		v := g.Next(r).(int64)
		if v < 1 || v > 1000 {
			t.Fatalf("out of range %d", v)
		}
		counts[v]++
	}
	if counts[1] < counts[500]*10 {
		t.Fatalf("zipf should favour low keys: key1=%d key500=%d", counts[1], counts[500])
	}
}

func TestChoiceWeights(t *testing.T) {
	g := must(t, Spec{Generator: "choice", Values: []any{"a", "b"}, Weights: []float64{9, 1}})
	r := Stream(3, "c")
	a := 0
	for n := 0; n < 10000; n++ {
		if g.Next(r) == "a" {
			a++
		}
	}
	if math.Abs(float64(a)/10000-0.9) > 0.02 {
		t.Fatalf("weight not respected: %d", a)
	}
}

func TestSequencePatternAndConst(t *testing.T) {
	seq := must(t, Spec{Generator: "sequence", Start: i(10), Step: i(5)})
	if seq.Next(nil) != int64(10) || seq.Next(nil) != int64(15) {
		t.Fatal("sequence values wrong")
	}
	p := must(t, Spec{Generator: "pattern", Format: "user{n}@example.test"})
	if p.Next(nil) != "user1@example.test" || p.Next(nil) != "user2@example.test" {
		t.Fatal("pattern values wrong")
	}
	if must(t, Spec{Generator: "const", Value: "x"}).Next(nil) != "x" {
		t.Fatal("const value wrong")
	}
}

func TestUUIDFormat(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	g := must(t, Spec{Generator: "uuid"})
	r := Stream(1, "u")
	for n := 0; n < 100; n++ {
		if s := g.Next(r).(string); !re.MatchString(s) {
			t.Fatalf("invalid uuid %s", s)
		}
	}
}

func TestTextTimestampBool(t *testing.T) {
	r := Stream(9, "t")
	tx := must(t, Spec{Generator: "text", MinLength: 3, MaxLength: 5})
	for n := 0; n < 50; n++ {
		if l := len(tx.Next(r).(string)); l < 3 || l > 5 {
			t.Fatalf("text length %d", l)
		}
	}
	ts := must(t, Spec{Generator: "timestamp_range", From: "2025-01-01T00:00:00Z", To: "2025-01-02T00:00:00Z"})
	from, _ := time.Parse(time.RFC3339, "2025-01-01T00:00:00Z")
	for n := 0; n < 50; n++ {
		v := ts.Next(r).(time.Time)
		if v.Before(from) || v.After(from.Add(24*time.Hour)) {
			t.Fatalf("timestamp out of range %v", v)
		}
	}
	b := must(t, Spec{Generator: "bool", TrueRatio: f(1)})
	if b.Next(r) != true {
		t.Fatal("true_ratio 1 must always be true")
	}
}

func TestNullRatioKeepsNonNullValuesStable(t *testing.T) {
	plain := must(t, Spec{Generator: "int_range", Min: f(1), Max: f(1e9)})
	nullable := must(t, Spec{Generator: "int_range", Min: f(1), Max: f(1e9), NullRatio: 0.5})
	r1, r2 := Stream(5, "n"), Stream(5, "n")
	nulls := 0
	for n := 0; n < 1000; n++ {
		want := plain.Next(r1)
		r1.Float64() // the nullable wrapper draws once more per value
		got := nullable.Next(r2)
		if got == nil {
			nulls++
			continue
		}
		if got != want {
			t.Fatalf("non-null value changed: %v vs %v", got, want)
		}
	}
	if nulls < 400 || nulls > 600 {
		t.Fatalf("null count %d far from 50%%", nulls)
	}
}

func TestValidateRejectsBadSpecs(t *testing.T) {
	bad := []Spec{
		{},
		{Generator: "nope"},
		{Generator: "int_range", Min: f(1)},
		{Generator: "int_range", Min: f(5), Max: f(1)},
		{Generator: "int_range", Min: f(1.5), Max: f(3)},
		{Generator: "zipf", Min: f(1), Max: f(10), S: 1},
		{Generator: "choice"},
		{Generator: "choice", Values: []any{1, 2}, Weights: []float64{1}},
		{Generator: "choice", Values: []any{1}, Weights: []float64{0}},
		{Generator: "sequence", Step: i(0)},
		{Generator: "text", MinLength: 5, MaxLength: 2},
		{Generator: "timestamp_range", From: "2025-01-02T00:00:00Z", To: "2025-01-01T00:00:00Z"},
		{Generator: "pattern", Format: "no placeholder"},
		{Generator: "bool", TrueRatio: f(2)},
		{Generator: "uuid", NullRatio: 1.5},
	}
	for _, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("expected validation error for %+v", s)
		}
	}
}
