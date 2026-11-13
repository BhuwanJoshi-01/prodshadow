package cli

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed templates
var templates embed.FS

var projectNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func runInit(_ context.Context, env *Env, args []string) int {
	fset := newFlags(env, "init", "[--dir DIR] [--project NAME] [--force]")
	dir := fset.String("dir", ".", "directory to create the files in")
	name := fset.String("project", "", "project name (default: directory name)")
	force := fset.Bool("force", false, "overwrite existing files")
	if code := parseFlags(fset, args); code >= 0 {
		return code
	}
	target := env.abs(*dir)
	if *name == "" {
		abs, _ := filepath.Abs(target)
		*name = filepath.Base(abs)
	}
	project := strings.Trim(projectNameRe.ReplaceAllString(*name, "-"), "-._")
	if project == "" {
		project = "my-service"
	}

	var created []string
	err := fs.WalkDir(templates, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(path, "templates/"), ".tmpl")
		dst := filepath.Join(target, filepath.FromSlash(rel))
		if _, err := os.Stat(dst); err == nil && !*force {
			return fmt.Errorf("%s already exists (use --force to overwrite)", display(env.Dir, dst))
		}
		raw, err := templates.ReadFile(path)
		if err != nil {
			return err
		}
		tpl, err := template.New(rel).Parse(string(raw))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if err := tpl.Execute(f, struct{ Project string }{project}); err != nil {
			f.Close()
			return err
		}
		created = append(created, display(env.Dir, dst))
		return f.Close()
	})
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			fmt.Fprintf(env.Stderr, "prodshadow: %v\n", pe)
		} else {
			fmt.Fprintf(env.Stderr, "prodshadow: %v\n", err)
		}
		return ExitInvalid
	}
	for _, c := range created {
		fmt.Fprintf(env.Stdout, "created %s\n", c)
	}
	fmt.Fprintln(env.Stdout, "\nNext steps:\n  prodshadow doctor\n  prodshadow inspect --config prodshadow.yaml\n  prodshadow verify --config prodshadow.yaml")
	return ExitOK
}
