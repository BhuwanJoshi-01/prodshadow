package gen

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
)

// Generator produces values for one column or parameter. Generators are
// stateful (sequences, patterns) and must not be shared between goroutines;
// build one per stream.
type Generator interface {
	Next(r *rand.Rand) any
}

// New builds a generator from a validated spec.
func New(s Spec) (Generator, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	var g Generator
	switch s.Generator {
	case "int_range":
		g = intRange{min: int64(*s.Min), max: int64(*s.Max)}
	case "float_range":
		g = floatRange{min: *s.Min, max: *s.Max}
	case "zipf":
		g = &zipf{min: int64(*s.Min), max: int64(*s.Max), s: s.S}
	case "choice":
		g = newChoice(s.Values, s.Weights)
	case "const":
		g = constant{v: s.Value}
	case "sequence":
		start, step := int64(1), int64(1)
		if s.Start != nil {
			start = *s.Start
		}
		if s.Step != nil {
			step = *s.Step
		}
		g = &sequence{next: start, step: step}
	case "text":
		g = text{min: s.MinLength, max: s.MaxLength}
	case "uuid":
		g = uuidGen{}
	case "timestamp_range":
		from, _ := time.Parse(time.RFC3339, s.From)
		to, _ := time.Parse(time.RFC3339, s.To)
		g = timeRange{from: from.UTC(), span: to.Sub(from)}
	case "pattern":
		g = &pattern{format: s.Format}
	case "bool":
		ratio := 0.5
		if s.TrueRatio != nil {
			ratio = *s.TrueRatio
		}
		g = boolGen{ratio: ratio}
	default:
		return nil, fmt.Errorf("unknown generator %q", s.Generator)
	}
	if s.NullRatio > 0 {
		g = nullable{inner: g, ratio: s.NullRatio}
	}
	return g, nil
}

type intRange struct{ min, max int64 }

func (g intRange) Next(r *rand.Rand) any { return g.min + r.Int64N(g.max-g.min+1) }

type floatRange struct{ min, max float64 }

func (g floatRange) Next(r *rand.Rand) any { return g.min + r.Float64()*(g.max-g.min) }

// zipf draws keys in [min, max] where key min is the most frequent.
type zipf struct {
	min, max int64
	s        float64
	z        *rand.Zipf
	src      *rand.Rand
}

func (g *zipf) Next(r *rand.Rand) any {
	if g.z == nil || g.src != r {
		// Validate guarantees max > min, so the span is positive.
		g.z, g.src = rand.NewZipf(r, g.s, 1, uint64(g.max-g.min)), r //nolint:gosec // span > 0
	}
	return g.min + int64(g.z.Uint64()) //nolint:gosec // bounded by the span
}

type choice struct {
	values []any
	cum    []float64
}

func newChoice(values []any, weights []float64) choice {
	c := choice{values: values}
	if len(weights) == 0 {
		return c
	}
	total := 0.0
	for _, w := range weights {
		total += w
		c.cum = append(c.cum, total)
	}
	for i := range c.cum {
		c.cum[i] /= total
	}
	return c
}

func (g choice) Next(r *rand.Rand) any {
	if g.cum == nil {
		return g.values[r.IntN(len(g.values))]
	}
	x := r.Float64()
	for i, c := range g.cum {
		if x < c {
			return g.values[i]
		}
	}
	return g.values[len(g.values)-1]
}

type constant struct{ v any }

func (g constant) Next(*rand.Rand) any { return g.v }

type sequence struct{ next, step int64 }

func (g *sequence) Next(*rand.Rand) any {
	v := g.next
	g.next += g.step
	return v
}

const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 "

type text struct{ min, max int }

func (g text) Next(r *rand.Rand) any {
	n := g.min
	if g.max > g.min {
		n += r.IntN(g.max - g.min + 1)
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[r.IntN(len(alphabet))]
	}
	return string(b)
}

// uuidGen produces RFC 4122 version 4 UUIDs from the deterministic stream.
type uuidGen struct{}

func (uuidGen) Next(r *rand.Rand) any {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], r.Uint64())
	binary.BigEndian.PutUint64(b[8:], r.Uint64())
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type timeRange struct {
	from time.Time
	span time.Duration
}

func (g timeRange) Next(r *rand.Rand) any {
	// Microsecond precision matches PostgreSQL timestamps.
	us := r.Int64N(int64(g.span/time.Microsecond) + 1)
	return g.from.Add(time.Duration(us) * time.Microsecond)
}

type pattern struct {
	format string
	n      int64
}

func (g *pattern) Next(*rand.Rand) any {
	g.n++
	return strings.ReplaceAll(g.format, "{n}", strconv.FormatInt(g.n, 10))
}

type boolGen struct{ ratio float64 }

func (g boolGen) Next(r *rand.Rand) any { return r.Float64() < g.ratio }

type nullable struct {
	inner Generator
	ratio float64
}

func (g nullable) Next(r *rand.Rand) any {
	// Always advance the inner generator so the non-null values do not
	// depend on the null ratio.
	v := g.inner.Next(r)
	if r.Float64() < g.ratio {
		return nil
	}
	return v
}
