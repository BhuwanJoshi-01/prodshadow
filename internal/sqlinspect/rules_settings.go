package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
)

func init() {
	register(Rule{
		ID:       "PS129",
		Name:     "server-setting-change",
		Summary:  "ALTER SYSTEM, ALTER DATABASE ... SET and ALTER ROLE ... SET change settings beyond the migration",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/config-setting.html",
		check:    checkServerSetting,
	})
}

// checkServerSetting flags persistent setting changes. Unlike SET, they
// outlive the migration's session and affect other sessions.
func checkServerSetting(_ *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var msg string
	switch {
	case s.node.GetAlterSystemStmt() != nil:
		name := s.node.GetAlterSystemStmt().GetSetstmt().GetName()
		msg = fmt.Sprintf("ALTER SYSTEM changes %s for the whole server through postgresql.auto.conf; it cannot run in a transaction block, is not rolled back with the migration, and takes effect on reload or restart, so the effect is separated from the deploy that caused it.", orAll(name))
	case s.node.GetAlterDatabaseSetStmt() != nil:
		st := s.node.GetAlterDatabaseSetStmt()
		msg = fmt.Sprintf("ALTER DATABASE %s SET changes %s for every new session of that database, including application connections opened after the migration.", st.Dbname, orAll(st.GetSetstmt().GetName()))
	case s.node.GetAlterRoleSetStmt() != nil:
		st := s.node.GetAlterRoleSetStmt()
		role := "ALL"
		if r := st.GetRole(); r != nil && r.Rolename != "" {
			role = r.Rolename
		}
		msg = fmt.Sprintf("ALTER ROLE %s SET changes %s for every new session of that role, including application connections opened after the migration.", role, orAll(st.GetSetstmt().GetName()))
	default:
		return nil
	}
	f := at(s, msg)
	f.Evidence, f.Confidence = evidence.Static, evidence.High
	f.Remediation = "Manage server, database and role settings in configuration management or the provider's parameter group, and use SET or SET LOCAL for settings the migration itself needs."
	return []Finding{f}
}

func orAll(name string) string {
	if name == "" {
		return "settings"
	}
	return name
}
