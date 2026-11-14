package sqlinspect

import (
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

func init() {
	register(Rule{
		ID:       "PS138",
		Name:     "privileges-revoked",
		Summary:  "REVOKE, DROP OWNED and DROP ROLE remove access that running application code may depend on",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-revoke.html",
		check:    checkPrivilegesRevoked,
	})
}

// checkPrivilegesRevoked flags statements that take privileges away. The
// migration cannot show which roles the application connects as, so the
// finding asks for a check rather than claiming breakage.
func checkPrivilegesRevoked(_ *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var msg string
	switch {
	case s.node.GetGrantStmt() != nil && !s.node.GetGrantStmt().IsGrant:
		st := s.node.GetGrantStmt()
		what := strings.ToLower(strings.TrimPrefix(st.Objtype.String(), "OBJECT_"))
		if st.Targtype == pg_query.GrantTargetType_ACL_TARGET_ALL_IN_SCHEMA {
			what = "all " + what + "s in schema"
		}
		msg = fmt.Sprintf("REVOKE on %s from %s takes effect immediately for every session, including application connections of the version currently deployed.", what, roleList(st.Grantees))
	case s.node.GetAlterDefaultPrivilegesStmt() != nil:
		if gs := s.node.GetAlterDefaultPrivilegesStmt().GetAction(); gs != nil && !gs.IsGrant {
			msg = fmt.Sprintf("ALTER DEFAULT PRIVILEGES ... REVOKE from %s means objects created later are not accessible to it, which surfaces only when the next table or sequence is created.", roleList(gs.Grantees))
		}
	case s.node.GetDropOwnedStmt() != nil:
		msg = fmt.Sprintf("DROP OWNED BY %s drops every object those roles own in this database and revokes all their privileges.", roleList(s.node.GetDropOwnedStmt().Roles))
	case s.node.GetDropRoleStmt() != nil:
		msg = fmt.Sprintf("DROP ROLE %s removes roles cluster-wide; connections that log in as them fail from now on.", roleList(s.node.GetDropRoleStmt().Roles))
	}
	if msg == "" {
		return nil
	}
	f := at(s, msg+" Make sure no running application, job or reporting user still relies on that access.")
	f.Evidence, f.Confidence = evidence.Heuristic, evidence.Medium
	f.Remediation = "Revoke access in a release after every client has stopped using it, and check pg_stat_activity for sessions of the affected roles first."
	return []Finding{f}
}

func roleList(nodes []*pg_query.Node) string {
	var names []string
	for _, n := range nodes {
		if r := n.GetRoleSpec(); r != nil {
			switch {
			case r.Rolename != "":
				names = append(names, r.Rolename)
			case r.Roletype == pg_query.RoleSpecType_ROLESPEC_PUBLIC:
				names = append(names, "PUBLIC")
			default:
				names = append(names, strings.ToUpper(strings.TrimPrefix(r.Roletype.String(), "ROLESPEC_")))
			}
		}
	}
	if len(names) == 0 {
		return "the named roles"
	}
	return strings.Join(names, ", ")
}
