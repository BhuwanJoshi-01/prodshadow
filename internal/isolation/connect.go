package isolation

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/security"

	"github.com/jackc/pgx/v5"
)

// ErrNotManaged is returned when a Server was not created by a provider.
var ErrNotManaged = errors.New("isolation: server is not a managed disposable server")

// ErrOwnership is returned when the server does not carry this run's token.
var ErrOwnership = errors.New("isolation: ownership token mismatch; refusing to use this server")

// ConnOptions customise a connection.
type ConnOptions struct {
	// Database to connect to; "postgres" when empty.
	Database string
	// ApplicationName is visible in pg_stat_activity and used to attribute
	// lock waits to workers and the migration session.
	ApplicationName string
	// ConnectTimeout bounds connection establishment.
	ConnectTimeout time.Duration
}

// Connect opens a session on a managed server. It refuses unmanaged servers
// and non-local addresses, and verifies the ownership token before
// returning the connection.
func (s *Server) Connect(ctx context.Context, cred Credential, opts ConnOptions) (*pgx.Conn, error) {
	cfg, err := s.connConfig(cred, opts)
	if err != nil {
		return nil, err
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to disposable server: %w", err)
	}
	if err := s.verifyOwnership(ctx, conn); err != nil {
		conn.Close(context.Background())
		return nil, err
	}
	return conn, nil
}

// connConfig builds a connection configuration from scratch. pgx would
// otherwise merge PG* environment variables (PGHOST, PGSERVICE, ...), which
// could silently redirect the connection; every relevant field is set or
// cleared explicitly here.
func (s *Server) connConfig(cred Credential, opts ConnOptions) (*pgx.ConnConfig, error) {
	if s == nil || !s.managed {
		return nil, ErrNotManaged
	}
	if err := security.CheckLocalAddress(s.host); err != nil {
		return nil, err
	}
	if cred.User == "" {
		return nil, errors.New("isolation: credential has no user")
	}
	db := opts.Database
	if db == "" {
		db = "postgres"
	}
	timeout := opts.ConnectTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	cfg, err := pgx.ParseConfig("sslmode=disable")
	if err != nil {
		return nil, err
	}
	cfg.Host = s.host
	cfg.Port = uint16(s.port)
	cfg.User = cred.User
	cfg.Password = cred.password
	cfg.Database = db
	cfg.TLSConfig = nil
	cfg.Fallbacks = nil
	cfg.ConnectTimeout = timeout
	cfg.KerberosSrvName, cfg.KerberosSpn = "", ""
	cfg.RuntimeParams = map[string]string{}
	if opts.ApplicationName != "" {
		cfg.RuntimeParams["application_name"] = opts.ApplicationName
	}
	cfg.DialFunc = guardedDial(&net.Dialer{Timeout: timeout})
	return cfg, nil
}

// guardedDial re-validates the destination at dial time, so that a
// configuration modified after construction still cannot reach a remote
// host.
func guardedDial(d *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if err := checkDialTarget(network, addr); err != nil {
			return nil, err
		}
		return d.DialContext(ctx, network, addr)
	}
}

func checkDialTarget(network, addr string) error {
	switch network {
	case "unix":
		dir := addr
		if i := strings.LastIndex(addr, "/"); i > 0 {
			dir = addr[:i]
		}
		return security.CheckLocalAddress(dir)
	case "tcp", "tcp4", "tcp6":
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("%w: %v", security.ErrUnmanagedTarget, err)
		}
		if _, err := strconv.Atoi(port); err != nil {
			return fmt.Errorf("%w: invalid port %q", security.ErrUnmanagedTarget, port)
		}
		return security.CheckLocalAddress(host)
	default:
		return fmt.Errorf("%w: network %q", security.ErrUnmanagedTarget, network)
	}
}

func (s *Server) verifyOwnership(ctx context.Context, conn *pgx.Conn) error {
	var got *string
	err := conn.QueryRow(ctx, "SELECT current_setting($1, true)", TokenSetting).Scan(&got)
	if err != nil {
		return fmt.Errorf("verify ownership token: %w", err)
	}
	if got == nil || *got != s.token {
		return ErrOwnership
	}
	return nil
}
