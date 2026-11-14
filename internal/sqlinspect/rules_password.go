package sqlinspect

import (
	"regexp"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func init() {
	register(Rule{
		ID:       "PS139",
		Name:     "plaintext-password",
		Summary:  "A role or user mapping password is written in the migration in plain text",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-createrole.html",
		check:    checkPlaintextPassword,
	})
}

// passwordVerifier matches values PostgreSQL stores as hashes rather than
// passwords: SCRAM verifiers and md5 hashes.
var passwordVerifier = regexp.MustCompile(`^(SCRAM-SHA-256\$|md5[0-9a-f]{32}$)`)

// checkPlaintextPassword flags CREATE/ALTER ROLE ... PASSWORD 'literal' and
// user mappings with a password option. Migration files live in version
// control, so the password is readable by everyone with the repository and
// stays in its history. The finding never repeats the value.
func checkPlaintextPassword(_ *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var where string
	switch {
	case s.node.GetCreateRoleStmt() != nil:
		if plaintextPassword(s.node.GetCreateRoleStmt().Options) {
			where = "CREATE ROLE " + s.node.GetCreateRoleStmt().Role
		}
	case s.node.GetAlterRoleStmt() != nil:
		st := s.node.GetAlterRoleStmt()
		if plaintextPassword(st.Options) {
			where = strings.TrimSpace("ALTER ROLE " + st.GetRole().GetRolename())
		}
	case s.node.GetCreateUserMappingStmt() != nil:
		if plaintextPassword(s.node.GetCreateUserMappingStmt().Options) {
			where = "CREATE USER MAPPING"
		}
	case s.node.GetAlterUserMappingStmt() != nil:
		if plaintextPassword(s.node.GetAlterUserMappingStmt().Options) {
			where = "ALTER USER MAPPING"
		}
	}
	if where == "" {
		return nil
	}
	f := at(s, where+" sets a password written in plain text in the migration. Everyone with access to the repository, its history and CI logs can read it, and it stays there after the password is changed.")
	f.Evidence, f.Confidence = evidence.Static, evidence.High
	f.Remediation = "Set passwords outside migrations (a secrets manager, or psql \\password, which sends a hash), or store a SCRAM verifier instead of the password."
	return []Finding{f}
}

// plaintextPassword reports whether options set a password to a string
// that is not a stored hash.
func plaintextPassword(options []*pg_query.Node) bool {
	found := false
	for _, o := range options {
		walk(o, func(m protoreflect.Message) bool {
			d, ok := m.Interface().(*pg_query.DefElem)
			if !ok || !strings.EqualFold(d.Defname, "password") || d.Arg == nil {
				return true
			}
			if v := d.Arg.GetString_(); v != nil && v.Sval != "" && !passwordVerifier.MatchString(v.Sval) {
				found = true
			}
			return false
		})
	}
	return found
}
