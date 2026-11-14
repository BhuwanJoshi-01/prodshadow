package isolation

import (
	"context"
	"fmt"

	"github.com/BhuwanJoshi-01/prodshadow/internal/security"

	"github.com/jackc/pgx/v5"
)

// AppUser is the unprivileged role that owns fixture objects and runs the
// workload and, by default, the migration. Running as a non-superuser
// means server-side file and program access (COPY ... PROGRAM,
// pg_read_file) is unavailable to fixture and migration SQL.
const AppUser = "ps_app"

// Bootstrap creates the application role with a fresh random password.
func (s *Server) Bootstrap(ctx context.Context, redactor *security.Redactor) error {
	if s == nil || !s.managed {
		return ErrNotManaged
	}
	password, err := security.RandomToken(24)
	if err != nil {
		return err
	}
	redactor.Add(password)
	conn, err := s.Connect(ctx, s.admin, ConnOptions{ApplicationName: "prodshadow/bootstrap"})
	if err != nil {
		return err
	}
	defer Close(conn)
	stmt := fmt.Sprintf("CREATE ROLE %s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %s",
		pgx.Identifier{AppUser}.Sanitize(), quoteLiteral(password))
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("create application role: %w", err)
	}
	s.app = &Credential{User: AppUser, password: password}
	return nil
}

// quoteLiteral quotes a string literal. Passwords are hex tokens, but the
// value is escaped regardless.
func quoteLiteral(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'')
		}
		out = append(out, s[i])
	}
	return string(append(out, '\''))
}
