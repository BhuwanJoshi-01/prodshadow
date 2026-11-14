package cli

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/internal/fixture"
	"github.com/BhuwanJoshi-01/prodshadow/internal/sqlinspect"
)

func runDraftFixture(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "draft-fixture", "--schema FILE [--rows FILE] [--scale F] [--default-rows N]")
	schema := fs.String("schema", "", "fixture schema SQL (CREATE TABLE statements)")
	rowsFile := fs.String("rows", "", "CSV with relname and reltuples columns, e.g. exported from pg_class")
	scale := fs.Float64("scale", 0.01, "multiply exported row counts by this factor")
	defaultRows := fs.Int64("default-rows", 10_000, "rows for tables without an exported count")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	if *schema == "" {
		fmt.Fprintln(env.Stderr, "prodshadow: --schema is required")
		return ExitInvalid
	}
	if *scale <= 0 || *defaultRows <= 0 {
		fmt.Fprintln(env.Stderr, "prodshadow: --scale and --default-rows must be positive")
		return ExitInvalid
	}
	src, err := sqlinspect.ReadSource(env.abs(*schema), *schema)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	stmts, err := sqlinspect.ParseSources([]sqlinspect.Source{src})
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	rows := map[string]int64{}
	if *rowsFile != "" {
		if rows, err = readRowCounts(env.abs(*rowsFile), *scale); err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: %s: %v\n", *rowsFile, err)
			return ExitInvalid
		}
	}
	d := fixture.DraftGenerator(stmts, fixture.DraftOptions{Rows: rows, DefaultRows: *defaultRows})
	if len(d.File.Tables) == 0 {
		fmt.Fprintln(env.Stderr, "prodshadow: the schema creates no table that can be generated")
		return ExitInvalid
	}
	if err := d.Write(env.Stdout); err != nil {
		return ExitInternal
	}
	return ExitOK
}

// readRowCounts reads table row counts from a CSV with a header naming a
// table column (relname, table or name) and a count column (reltuples or
// rows), and scales them. Scaled counts are at least 100.
func readRowCounts(path string, scale float64) (map[string]int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(io.LimitReader(f, maxStatsFile))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	ti, ci := -1, -1
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "relname", "table", "name":
			ti = i
		case "reltuples", "rows":
			ci = i
		}
	}
	if ti < 0 || ci < 0 {
		return nil, errors.New("the header must name a table column (relname) and a count column (reltuples)")
	}
	out := map[string]int64{}
	for line := 2; ; line++ {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if ti >= len(row) || ci >= len(row) {
			return nil, fmt.Errorf("row %d has %d columns", line, len(row))
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(row[ci]), 64)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("row %d: count %q is not a non-negative number", line, row[ci])
		}
		out[strings.TrimSpace(row[ti])] = max(100, int64(math.Round(n*scale)))
	}
}
