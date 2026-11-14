package fixture

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/internal/gen"

	"github.com/jackc/pgx/v5"
)

// LoadResult records how many rows were generated per table.
type LoadResult struct {
	Table string `json:"table"`
	Rows  int64  `json:"rows"`
}

// Generate streams synthetic rows into existing tables with COPY. Values
// depend only on seed, table and column names, so repeated runs produce
// byte-identical data.
func Generate(ctx context.Context, conn *pgx.Conn, gf *GeneratorFile, seed int64) ([]LoadResult, error) {
	var out []LoadResult
	for _, t := range gf.Tables {
		n, err := generateTable(ctx, conn, t, seed)
		if err != nil {
			return out, fmt.Errorf("generate %s: %w", t.Name, err)
		}
		out = append(out, LoadResult{Table: t.Name, Rows: n})
	}
	return out, nil
}

func generateTable(ctx context.Context, conn *pgx.Conn, t TableSpec, seed int64) (int64, error) {
	cols := make([]string, len(t.Columns))
	gens := make([]gen.Generator, len(t.Columns))
	streams := make([]*rand.Rand, len(t.Columns))
	for i, c := range t.Columns {
		g, err := gen.New(c.Spec)
		if err != nil {
			return 0, fmt.Errorf("column %s: %w", c.Name, err)
		}
		cols[i], gens[i] = c.Name, g
		streams[i] = gen.Stream(seed, "fixture/"+t.Name+"/"+c.Name)
	}

	var produced int64
	row := make([]any, len(cols))
	src := pgx.CopyFromFunc(func() ([]any, error) {
		if produced >= t.Rows {
			return nil, nil
		}
		produced++
		for i, g := range gens {
			row[i] = g.Next(streams[i])
		}
		return row, nil
	})
	return conn.CopyFrom(ctx, tableIdentifier(t.Name), cols, src)
}

func tableIdentifier(name string) pgx.Identifier {
	return pgx.Identifier(strings.Split(name, "."))
}
