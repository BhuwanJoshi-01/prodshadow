package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS125",
		Name:     "integrity-checks-disabled",
		Summary:  "session_replication_role = replica and DISABLE TRIGGER ALL switch off foreign key enforcement",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/runtime-config-client.html#GUC-SESSION-REPLICATION-ROLE",
		check:    checkIntegrityDisabled,
	})
}

// checkIntegrityDisabled flags statements that turn off the internal
// triggers PostgreSQL uses to enforce foreign keys. Rows written while they
// are off are never re-checked.
func checkIntegrityDisabled(c *Context, s *Statement) []Finding {
	if name, value, ok := settingValue(s); ok && name == "session_replication_role" && strings.EqualFold(value, "replica") {
		f := at(s, "SET session_replication_role = replica disables all ordinary triggers, including the ones that enforce foreign keys, for the rest of the session or transaction. Rows written meanwhile are never checked, and later VALIDATE CONSTRAINT does not re-check foreign keys that are already valid.")
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Load data in an order that satisfies the foreign keys, or add the constraint NOT VALID and VALIDATE it after the load so violations are reported."
		return []Finding{f}
	}
	table, cmds := alterCmds(c, s)
	var out []Finding
	for _, cmd := range cmds {
		if cmd.Subtype != pg_query.AlterTableType_AT_DisableTrigAll {
			continue
		}
		f := at(s, fmt.Sprintf("DISABLE TRIGGER ALL on %s also disables the internal triggers that enforce foreign keys on it; rows written until the triggers are enabled again are never checked.", table))
		f.Table = table
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Disable only the user triggers you need to (DISABLE TRIGGER USER or by name), or drop and re-add the foreign key as NOT VALID followed by VALIDATE CONSTRAINT."
		out = append(out, f)
	}
	return out
}
