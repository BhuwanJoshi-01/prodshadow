package gen

import "testing"

func BenchmarkGenerators(b *testing.B) {
	specs := map[string]Spec{
		"int_range": {Generator: "int_range", Min: f(1), Max: f(1e6)},
		"zipf":      {Generator: "zipf", Min: f(1), Max: f(1e6), S: 1.1},
		"uuid":      {Generator: "uuid"},
		"text":      {Generator: "text", MinLength: 10, MaxLength: 40},
	}
	for name, s := range specs {
		b.Run(name, func(b *testing.B) {
			g, err := New(s)
			if err != nil {
				b.Fatal(err)
			}
			r := Stream(1, name)
			for b.Loop() {
				_ = g.Next(r)
			}
		})
	}
}
