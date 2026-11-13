package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// MaxFileSize bounds configuration files to protect against accidental
// loading of large or binary files.
const MaxFileSize = 1 << 20

// ErrEmpty is returned when the configuration document is empty.
var ErrEmpty = errors.New("configuration is empty")

// Load reads path, parses it and resolves relative paths against the file's
// directory. Defaults are applied. The returned Problems hold unknown keys
// and type mismatches; a non-nil error means the file could not be read or
// is not valid YAML.
func Load(path string) (*Config, Problems, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if fi.IsDir() {
		return nil, nil, fmt.Errorf("%s is a directory", path)
	}
	if fi.Size() > MaxFileSize {
		return nil, nil, fmt.Errorf("%s is larger than %d bytes", path, MaxFileSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	cfg, problems, err := Parse(data, filepath.Dir(abs))
	if cfg != nil {
		cfg.path = abs
	}
	return cfg, problems, err
}

// Parse decodes a configuration document. baseDir is used to resolve
// relative file references.
func Parse(data []byte, baseDir string) (*Config, Problems, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("parse configuration: %w", err)
	}
	if root.Kind == 0 || len(root.Content) == 0 {
		return nil, nil, ErrEmpty
	}
	if root.Content[0].Kind != yaml.MappingNode {
		return nil, nil, errors.New("parse configuration: top level must be a mapping")
	}

	idx := newKeyIndex()
	idx.walk(&root, reflect.TypeOf(Config{}), "")
	problems := append(Problems(nil), idx.unknown...)

	cfg := &Config{baseDir: baseDir, lines: idx.lines}
	if err := root.Decode(cfg); err != nil {
		var te *yaml.TypeError
		if !errors.As(err, &te) {
			return nil, problems, fmt.Errorf("decode configuration: %w", err)
		}
		for _, msg := range te.Errors {
			problems = append(problems, typeProblem(msg))
		}
	}
	cfg.ApplyDefaults()
	return cfg, problems, nil
}

var typeErrLine = regexp.MustCompile(`^line (\d+): (.*)$`)

func typeProblem(msg string) Problem {
	p := Problem{Level: LevelError, Path: "(document)", Message: msg}
	if m := typeErrLine.FindStringSubmatch(msg); m != nil {
		p.Line, _ = strconv.Atoi(m[1])
		p.Message = m[2]
	}
	return p
}

// Path returns the absolute path of the loaded file, if any.
func (c *Config) Path() string { return c.path }

// BaseDir returns the directory used to resolve relative references.
func (c *Config) BaseDir() string { return c.baseDir }

// Resolve makes p absolute relative to the configuration directory.
func (c *Config) Resolve(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(c.baseDir, p))
}

// LineOf returns the source line of a dotted configuration path, or 0.
func (c *Config) LineOf(path string) int {
	if c.lines == nil {
		return 0
	}
	return c.lines[path]
}
