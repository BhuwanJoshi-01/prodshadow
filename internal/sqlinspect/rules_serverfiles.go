package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func init() {
	register(Rule{
		ID:       "PS131",
		Name:     "server-file-access",
		Summary:  "COPY with a server file or PROGRAM, and file-reading functions, run on the database server's filesystem",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/sql-copy.html",
		check:    checkServerFileAccess,
	})
}

// serverFileFunctions read or write files on the database server.
var serverFileFunctions = map[string]bool{
	"pg_read_file": true, "pg_read_binary_file": true, "pg_stat_file": true, "pg_ls_dir": true,
	"lo_import": true, "lo_export": true,
}

func checkServerFileAccess(_ *Context, s *Statement) []Finding {
	if s.node == nil {
		return nil
	}
	var what string
	if cp := s.node.GetCopyStmt(); cp != nil && cp.Filename != "" {
		dir := "reads"
		if !cp.IsFrom {
			dir = "writes"
		}
		switch {
		case cp.IsProgram:
			what = fmt.Sprintf("COPY ... PROGRAM runs %q as the database server's operating system user (it needs superuser or pg_execute_server_program)", cp.Filename)
		default:
			what = fmt.Sprintf("COPY %s the file %q on the database server, not on the machine running the migration (it needs superuser or pg_%s_server_files)", dir, cp.Filename, map[bool]string{true: "read", false: "write"}[cp.IsFrom])
		}
	} else {
		walk(s.node, func(m protoreflect.Message) bool {
			if fc, ok := m.Interface().(*pg_query.FuncCall); ok && what == "" {
				if name := lastName(qualifiedName(fc.Funcname)); serverFileFunctions[name] {
					what = name + "() accesses the database server's filesystem (it needs superuser or an explicit grant)"
				}
			}
			return true
		})
	}
	if what == "" {
		return nil
	}
	f := at(s, what+". Managed services do not allow it, the files differ between environments, and the migration role should not hold these privileges.")
	f.Evidence, f.Confidence = evidence.Static, evidence.High
	f.Remediation = "Load data from the client side instead (psql \\copy or COPY ... FROM STDIN from the deploy job), or ship it as SQL in the migration."
	return []Finding{f}
}
