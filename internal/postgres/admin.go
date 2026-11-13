// Package postgres contains administrative and introspection helpers that
// operate on connections obtained from the isolation package. It never
// creates connections itself.
package postgres

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
)

var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

func ident(name string) (string, error) {
	if !identRe.MatchString(name) {
		return "", fmt.Errorf("invalid identifier %q", name)
	}
	return pgx.Identifier{name}.Sanitize(), nil
}

// CreateDatabase creates name owned by owner, optionally from a template.
func CreateDatabase(ctx context.Context, conn *pgx.Conn, name, owner, template string) error {
	db, err := ident(name)
	if err != nil {
		return err
	}
	own, err := ident(owner)
	if err != nil {
		return err
	}
	stmt := "CREATE DATABASE " + db + " OWNER " + own
	if template != "" {
		tpl, err := ident(template)
		if err != nil {
			return err
		}
		stmt += " TEMPLATE " + tpl
	}
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("create database %s: %w", name, err)
	}
	return nil
}

// DropDatabase drops name, terminating remaining sessions (PostgreSQL 13+).
func DropDatabase(ctx context.Context, conn *pgx.Conn, name string) error {
	db, err := ident(name)
	if err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+db+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("drop database %s: %w", name, err)
	}
	return nil
}

// CreateExtension installs an extension in the connected database.
func CreateExtension(ctx context.Context, conn *pgx.Conn, name string) error {
	ext, err := ident(name)
	if err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+ext); err != nil {
		return fmt.Errorf("create extension %s: %w", name, err)
	}
	return nil
}

// Checkpoint forces a checkpoint so that each measured phase starts from a
// comparable WAL and buffer state.
func Checkpoint(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, "CHECKPOINT")
	return err
}
