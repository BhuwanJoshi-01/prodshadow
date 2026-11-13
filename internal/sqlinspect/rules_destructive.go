package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS109",
		Name:     "destructive-change",
		Summary:  "DROP TABLE, DROP COLUMN and TRUNCATE remove data and need explicit review",
		Severity: evidence.Warning,
		DocsURL:  docDropTable,
		check:    checkDestructive,
	})
	register(Rule{
		ID:       "PS110",
		Name:     "rename-breaks-running-code",
		Summary:  "Renaming a table or column breaks application instances that still use the old name",
		Severity: evidence.Warning,
		DocsURL:  docAlterTable,
		check:    checkRename,
	})
}

func checkDestructive(c *Context, s *Statement) []Finding {
	var out []Finding
	switch s.Kind {
	case KindDropTable, KindTruncate:
		verb := "Dropping"
		if s.Kind == KindTruncate {
			verb = "Truncating"
		}
		for _, l := range s.Locks {
			if c.CreatedEarlier(s, l.Table) {
				continue
			}
			f := at(s, fmt.Sprintf("%s %s permanently removes its data and takes ACCESS EXCLUSIVE; application versions that still query it will fail.", verb, l.Table))
			f.Table, f.Lock = l.Table, pglock.AccessExclusive
			f.Remediation = "Confirm no deployed version reads the table, keep a backup or rename-and-wait period, and drop it in a later release."
			out = append(out, f)
		}
	case KindAlterTable:
		table, cmds := alterCmds(c, s)
		for _, cmd := range cmds {
			if cmd.Subtype != pg_query.AlterTableType_AT_DropColumn {
				continue
			}
			f := at(s, fmt.Sprintf("Dropping column %s.%s removes its data; application versions that still select or insert it will fail during a rolling deploy.", table, cmd.Name))
			f.Table, f.Lock = table, pglock.AccessExclusive
			f.Remediation = "Stop reading and writing the column in a release that is fully deployed first, then drop it."
			out = append(out, f)
		}
	}
	return out
}

func checkRename(c *Context, s *Statement) []Finding {
	if s.Kind != KindRename || len(s.Subcommands) == 0 {
		return nil
	}
	sub := s.Subcommands[0]
	if sub != "rename_column" && sub != "rename_table" {
		return nil
	}
	table := firstTable(s)
	if c.CreatedEarlier(s, table) {
		return nil
	}
	what := strings.TrimPrefix(sub, "rename_")
	f := at(s, fmt.Sprintf("Renaming a %s on %s is instant but breaks every running application instance that still uses the old name until all instances are replaced.", what, table))
	f.Table, f.Lock = table, pglock.AccessExclusive
	f.Remediation = "Use expand and contract: add the new name, write to both, migrate readers, then remove the old name in a later release."
	return []Finding{f}
}
