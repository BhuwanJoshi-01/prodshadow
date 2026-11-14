package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS144",
		Name:     "row-security-disabled",
		Summary:  "Disabling row-level security or dropping a policy exposes rows the policy used to hide",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/ddl-rowsecurity.html",
		check:    checkRowSecurityDisabled,
	})
	register(Rule{
		ID:       "PS145",
		Name:     "granted-to-public",
		Summary:  "Privileges granted to PUBLIC apply to every role, including ones created later",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-grant.html",
		check:    checkGrantToPublic,
	})
}

func checkRowSecurityDisabled(c *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	if dp := s.node.GetDropStmt(); dp != nil && dp.RemoveType == pg_query.ObjectType_OBJECT_POLICY {
		var out []Finding
		for _, o := range dp.Objects {
			name := strings.Join(stringItems(o.GetList().GetItems()), ".")
			f := at(s, fmt.Sprintf("DROP POLICY %s removes a row-level security rule: unless another policy covers the same rows, they become visible or writable to roles the policy used to restrict.", name))
			f.Evidence, f.Confidence = evidence.Heuristic, evidence.Medium
			f.Remediation = "Create the replacement policy before dropping the old one, in the same transaction."
			out = append(out, f)
		}
		return out
	}
	table, cmds := alterCmds(c, s)
	var out []Finding
	for _, cmd := range cmds {
		var what string
		switch cmd.Subtype {
		case pg_query.AlterTableType_AT_DisableRowSecurity:
			what = "DISABLE ROW LEVEL SECURITY turns off every policy on %s: all roles with table privileges see and change every row."
		case pg_query.AlterTableType_AT_NoForceRowSecurity:
			what = "NO FORCE ROW LEVEL SECURITY exempts the owner of %s from its policies again."
		default:
			continue
		}
		f := at(s, fmt.Sprintf(what, table))
		f.Table = table
		f.Evidence, f.Confidence = evidence.Static, evidence.High
		f.Remediation = "Keep row-level security enabled and change the policies instead; if it must be off for a data fix, enable it again in the same transaction."
		out = append(out, f)
	}
	return out
}

func checkGrantToPublic(_ *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var grantees []*pg_query.Node
	var what string
	switch {
	case s.node.GetGrantStmt() != nil && s.node.GetGrantStmt().IsGrant:
		st := s.node.GetGrantStmt()
		grantees = st.Grantees
		what = strings.ToLower(strings.TrimPrefix(st.Objtype.String(), "OBJECT_"))
		if st.Targtype == pg_query.GrantTargetType_ACL_TARGET_ALL_IN_SCHEMA {
			what = "all " + what + "s in a schema"
		}
	case s.node.GetAlterDefaultPrivilegesStmt() != nil && s.node.GetAlterDefaultPrivilegesStmt().GetAction().GetIsGrant():
		grantees = s.node.GetAlterDefaultPrivilegesStmt().GetAction().Grantees
		what = "objects created from now on"
	default:
		return nil
	}
	for _, g := range grantees {
		if r := g.GetRoleSpec(); r != nil && r.Roletype == pg_query.RoleSpecType_ROLESPEC_PUBLIC {
			f := at(s, fmt.Sprintf("GRANT ... TO PUBLIC on %s gives the privilege to every role in the cluster, including roles created later and roles of other applications sharing the server.", what))
			f.Evidence, f.Confidence = evidence.Static, evidence.High
			f.Remediation = "Grant to the specific application or group roles that need the access."
			return []Finding{f}
		}
	}
	return nil
}
