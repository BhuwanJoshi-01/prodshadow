package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BhuwanJoshi-01/prodshadow/internal/report"
)

func runHistory(_ context.Context, env *Env, args []string) int {
	flags := newFlags(env, "history", "[--project NAME] [--since DATE] [--last N] [--json | --markdown | --html] REPORT|DIR ...")
	project := flags.String("project", "", "only include runs of this project")
	last := flags.Int("last", 0, "only show the N most recent runs (0 shows all)")
	since := flags.String("since", "", "only runs started on or after this date (YYYY-MM-DD, UTC)")
	asJSON := flags.Bool("json", false, "print machine-readable JSON")
	asMarkdown := flags.Bool("markdown", false, "print a Markdown table, e.g. for a job summary")
	asHTML := flags.Bool("html", false, "print a self-contained HTML page with charts")
	paths, code := parseWithArgs(flags, args)
	if code >= 0 {
		return code
	}
	if len(paths) == 0 {
		fmt.Fprintln(env.Stderr, "prodshadow: name at least one report file or directory of reports")
		return ExitInvalid
	}
	if *last < 0 {
		fmt.Fprintln(env.Stderr, "prodshadow: --last must not be negative")
		return ExitInvalid
	}
	var from time.Time
	if *since != "" {
		t, err := time.Parse(time.DateOnly, *since)
		if err != nil {
			fmt.Fprintf(env.Stderr, "prodshadow: --since %q must be a date in YYYY-MM-DD form\n", *since)
			return ExitInvalid
		}
		from = t
	}
	entries, err := loadHistory(env, paths)
	if err != nil {
		fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		return ExitInvalid
	}
	kept := entries[:0]
	for _, e := range entries {
		if (*project == "" || e.Project == *project) && !e.StartedAt.Before(from) {
			kept = append(kept, e)
		}
	}
	entries = kept
	report.SortHistory(entries)
	if *last > 0 && len(entries) > *last {
		entries = entries[len(entries)-*last:]
	}
	if *asJSON {
		if entries == nil {
			entries = []report.HistoryEntry{}
		}
		b, _ := json.MarshalIndent(entries, "", "  ")
		fmt.Fprintln(env.Stdout, string(b))
		return ExitOK
	}
	if len(entries) == 0 {
		fmt.Fprintln(env.Stdout, "No reports found.")
		return ExitOK
	}
	write := report.WriteHistory
	switch {
	case *asMarkdown:
		write = report.WriteHistoryMarkdown
	case *asHTML:
		write = report.WriteHistoryHTML
	}
	if err := write(env.Stdout, entries); err != nil {
		return ExitInternal
	}
	return ExitOK
}

// loadHistory reads report files. A directory contributes every *.json file
// directly inside it; files there that are not ProdShadow reports are
// skipped with a note, while a named file that is not a report is an error.
func loadHistory(env *Env, paths []string) ([]report.HistoryEntry, error) {
	var out []report.HistoryEntry
	for _, p := range paths {
		abs := env.abs(p)
		fi, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			r, err := report.ReadFile(abs)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			out = append(out, report.HistoryOf(r, p))
			continue
		}
		names, err := filepath.Glob(filepath.Join(abs, "*.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(names)
		for _, n := range names {
			r, err := report.ReadFile(n)
			if err != nil || r.RunID == "" {
				if err == nil {
					err = errors.New("no run_id")
				}
				fmt.Fprintf(env.Stderr, "skipping %s: not a ProdShadow report (%v)\n", filepath.Join(p, filepath.Base(n)), shortErr(err))
				continue
			}
			out = append(out, report.HistoryOf(r, filepath.Join(p, filepath.Base(n))))
		}
	}
	return out, nil
}

func shortErr(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	msg := err.Error()
	if i := strings.Index(msg, "\n"); i >= 0 {
		msg = msg[:i]
	}
	return msg
}
