package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/BhuwanJoshi-01/prodshadow/internal/pglock"
)

type lockInfo struct {
	Mode        string   `json:"mode"`
	PgLocks     string   `json:"pg_locks_mode"`
	Impact      string   `json:"impact"`
	TakenBy     []string `json:"taken_by"`
	Conflicts   []string `json:"conflicts_with"`
	BlocksWhile []string `json:"blocks"`
}

func describeLock(m pglock.Mode) lockInfo {
	info := lockInfo{Mode: m.String(), PgLocks: m.PgLocksName(), Impact: string(m.Impact()), TakenBy: m.AcquiredBy(), BlocksWhile: m.BlockedStatements()}
	for _, c := range m.ConflictingModes() {
		info.Conflicts = append(info.Conflicts, c.String())
	}
	return info
}

func runLocks(_ context.Context, env *Env, args []string) int {
	fs := newFlags(env, "locks", "[--json] [MODE]")
	asJSON := fs.Bool("json", false, "print machine-readable JSON")
	modes, code := parseWithArgs(fs, args)
	if code >= 0 {
		return code
	}
	selected := pglock.Modes()
	if len(modes) > 0 {
		// Accept "access exclusive", "ACCESS_EXCLUSIVE", "access-exclusive"
		// or "AccessExclusiveLock".
		name := strings.NewReplacer("_", " ", "-", " ").Replace(strings.Join(modes, " "))
		m, err := pglock.Parse(name)
		if err != nil || m == pglock.None {
			fmt.Fprintf(env.Stderr, "prodshadow: unknown lock mode %q\n", strings.Join(modes, " "))
			return ExitInvalid
		}
		selected = []pglock.Mode{m}
	}
	infos := make([]lockInfo, 0, len(selected))
	for _, m := range selected {
		infos = append(infos, describeLock(m))
	}
	if *asJSON {
		b, _ := json.MarshalIndent(infos, "", "  ")
		fmt.Fprintln(env.Stdout, string(b))
		return ExitOK
	}
	if len(infos) == 1 {
		printLock(env.Stdout, infos[0])
		return ExitOK
	}
	tw := tabwriter.NewWriter(env.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "MODE\tTAKEN BY\tBLOCKS")
	for _, i := range infos {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", i.Mode, strings.Join(i.TakenBy, ", "), orNone(strings.Join(i.BlocksWhile, ", ")))
	}
	if err := tw.Flush(); err != nil {
		return ExitInternal
	}
	fmt.Fprintln(env.Stdout, "\nA lock that is queued blocks the same statements as one that is held, because later requests wait behind it.")
	return ExitOK
}

func printLock(w io.Writer, i lockInfo) {
	fmt.Fprintf(w, "%s (%s in pg_locks)\n\n", i.Mode, i.PgLocks)
	fmt.Fprintf(w, "  taken by:       %s\n", orNone(strings.Join(i.TakenBy, ", ")))
	fmt.Fprintf(w, "  conflicts with: %s\n", strings.Join(i.Conflicts, ", "))
	fmt.Fprintf(w, "  blocks:         %s\n", orNone(strings.Join(i.BlocksWhile, ", ")))
	fmt.Fprintf(w, "  impact:         %s\n", i.Impact)
	fmt.Fprintln(w, "\nWhile it is held, or queued behind another lock, the statements above wait.")
}

func orNone(s string) string {
	if s == "" {
		return "–"
	}
	return s
}
