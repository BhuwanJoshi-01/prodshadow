package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/config"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"
)

// DefaultConfig is the configuration file used when --config is omitted.
const DefaultConfig = "prodshadow.yaml"

// project is a loaded and validated configuration.
type project struct {
	cfg        *config.Config
	problems   config.Problems
	envApplied []string
}

func (env *Env) abs(p string) string {
	if p == "" || filepath.IsAbs(p) || env.Dir == "" {
		return p
	}
	return filepath.Join(env.Dir, p)
}

// loadProject loads the configuration, applies environment overrides and
// validates it. A non-nil error means the file could not be read at all.
func loadProject(env *Env, path string) (*project, error) {
	cfg, problems, err := config.Load(env.abs(path))
	if err != nil {
		return nil, err
	}
	applied, envProblems := cfg.ApplyEnv(env.Getenv)
	problems = append(problems, envProblems...)
	problems = append(problems, cfg.Validate()...)
	return &project{cfg: cfg, problems: problems, envApplied: applied}, nil
}

// display returns a path relative to base when possible, for reports.
func display(base, p string) string {
	if rel, err := filepath.Rel(base, p); err == nil && !filepath.IsAbs(rel) && rel != "" && rel[0] != '.' {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}

// expandMigrations resolves entries relative to base and expands glob
// patterns such as migrations/*.sql in lexical order, which matches the
// numbering convention of migration tools. A pattern without matches is an
// error, so a typo cannot silently test nothing.
func expandMigrations(base string, files []string) ([]string, error) {
	var out []string
	for _, f := range files {
		abs := f
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(base, f)
		}
		if !strings.ContainsAny(f, "*?[") {
			out = append(out, abs)
			continue
		}
		matches, err := filepath.Glob(abs)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", f, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("pattern %q matches no files", f)
		}
		sort.Strings(matches)
		out = append(out, matches...)
	}
	return out, nil
}

// loadMigrations reads migration files in order and parses them.
func loadMigrations(base string, files []string) ([]*sqlinspect.Statement, error) {
	if len(files) == 0 {
		return nil, errors.New("no migration files: pass --migration or set migration.files")
	}
	paths, err := expandMigrations(base, files)
	if err != nil {
		return nil, err
	}
	srcs := make([]sqlinspect.Source, 0, len(paths))
	for _, abs := range paths {
		src, err := sqlinspect.ReadSource(abs, display(base, abs))
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, src)
	}
	return sqlinspect.ParseSources(srcs)
}

// fixtureCatalog learns indexes, partitions, views, sequences and foreign
// keys from the fixture schema, or returns nil when there is no readable
// schema (validate reports that).
func fixtureCatalog(cfg *config.Config) *sqlinspect.Catalog {
	if cfg == nil || cfg.Fixture.Schema == "" {
		return nil
	}
	schema, err := schemaStatements(cfg.Resolve(cfg.Fixture.Schema), cfg.Fixture.Schema)
	if err != nil {
		return nil
	}
	return sqlinspect.NewCatalog(schema)
}

// schemaStatements parses a schema file (path on disk, display name for
// messages).
func schemaStatements(path, display string) ([]*sqlinspect.Statement, error) {
	src, err := sqlinspect.ReadSource(path, display)
	if err != nil {
		return nil, err
	}
	return sqlinspect.ParseSources([]sqlinspect.Source{src})
}

// resolveFromFixture adds the locks migration statements take on relations
// they do not name (an index's table, the partitions of a partitioned
// table, the tables behind a materialized view), learned from the fixture
// schema, and returns a catalog of the schema as it was before the
// migration for the static rules. Without a readable schema it changes
// nothing and returns nil.
func resolveFromFixture(cfg *config.Config, stmts []*sqlinspect.Statement) *sqlinspect.Catalog {
	// Resolve learns from the migration as it goes, so the rules get a
	// catalog of their own.
	fixtureCatalog(cfg).Resolve(stmts)
	return fixtureCatalog(cfg)
}

func printProblems(w io.Writer, ps config.Problems) {
	for _, p := range ps.Sorted() {
		fmt.Fprintln(w, p.String())
	}
}
