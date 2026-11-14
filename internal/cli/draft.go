package cli

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

// maxStatsFile bounds the statistics export read by draft-workload.
const maxStatsFile = 32 << 20

func runDraftWorkload(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "draft-workload", "--stats FILE [--table NAME] [--top N] [--name NAME]")
	stats := fs.String("stats", "", "CSV export of pg_stat_statements with at least the columns query and calls")
	table := fs.String("table", "", "only statements that mention this table")
	top := fs.Int("top", 20, "keep the N most frequent statements (0 keeps all)")
	name := fs.String("name", "", "workload name")
	if code := parseFlags(fs, args); code >= 0 {
		return code
	}
	if *stats == "" {
		fmt.Fprintln(env.Stderr, "prodshadow: --stats is required")
		return ExitInvalid
	}
	if *top < 0 {
		fmt.Fprintln(env.Stderr, "prodshadow: --top must not be negative")
		return ExitInvalid
	}
	records, err := readStatsCSV(env.abs(*stats))
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %s: %v\n", *stats, err)
		return ExitInvalid
	}
	d := workload.DraftFromStats(records, workload.DraftOptions{Top: *top, Table: *table, Name: *name})
	if err := d.Write(env.Stdout); err != nil {
		return ExitInternal
	}
	if len(d.Scenarios) == 0 {
		fmt.Fprintln(env.Stderr, "prodshadow: no statement in the export can be used as a scenario")
		return ExitInvalid
	}
	return ExitOK
}

// readStatsCSV reads a CSV file with a header row containing query and
// calls columns, in any order and case; other columns are ignored.
func readStatsCSV(path string) ([]workload.StatRecord, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > maxStatsFile {
		return nil, fmt.Errorf("larger than %d bytes", maxStatsFile)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	qi, ci := -1, -1
	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "query":
			qi = i
		case "calls":
			ci = i
		}
	}
	if qi < 0 || ci < 0 {
		return nil, errors.New("the header must name a query and a calls column")
	}
	var out []workload.StatRecord
	for line := 2; ; line++ {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if qi >= len(row) || ci >= len(row) {
			return nil, fmt.Errorf("row %d has %d columns", line, len(row))
		}
		calls, err := strconv.ParseInt(strings.TrimSpace(row[ci]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: calls %q is not an integer", line, row[ci])
		}
		out = append(out, workload.StatRecord{Query: row[qi], Calls: calls})
	}
}
