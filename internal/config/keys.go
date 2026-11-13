package config

import (
	"reflect"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// keyIndex records the source line of every key seen in the document and
// reports keys that do not exist in the typed model.
type keyIndex struct {
	lines   map[string]int
	unknown Problems
}

func newKeyIndex() *keyIndex {
	return &keyIndex{lines: map[string]int{}}
}

// walk compares a YAML node with the Go type it will be decoded into.
func (k *keyIndex) walk(node *yaml.Node, t reflect.Type, path string) {
	if node == nil {
		return
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, c := range node.Content {
			k.walk(c, t, path)
		}
	case yaml.AliasNode:
		k.walk(node.Alias, t, path)
	case yaml.MappingNode:
		switch t.Kind() {
		case reflect.Struct:
			fields := yamlFields(t)
			for i := 0; i+1 < len(node.Content); i += 2 {
				key, val := node.Content[i], node.Content[i+1]
				p := joinPath(path, key.Value)
				k.lines[p] = key.Line
				ft, ok := fields[key.Value]
				if !ok {
					msg := "unknown key " + strconv.Quote(key.Value)
					if s := suggest(key.Value, fields); s != "" {
						msg += "; did you mean " + strconv.Quote(s) + "?"
					}
					k.unknown = append(k.unknown, Problem{Level: LevelWarning, Path: p, Message: msg, Line: key.Line})
					continue
				}
				k.walk(val, ft, p)
			}
		case reflect.Map:
			for i := 0; i+1 < len(node.Content); i += 2 {
				key, val := node.Content[i], node.Content[i+1]
				p := joinPath(path, key.Value)
				k.lines[p] = key.Line
				k.walk(val, t.Elem(), p)
			}
		}
	case yaml.SequenceNode:
		if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
			for i, c := range node.Content {
				p := path + "[" + strconv.Itoa(i) + "]"
				k.lines[p] = c.Line
				k.walk(c, t.Elem(), p)
			}
		}
	}
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// yamlFields maps yaml tag names of exported fields to their types.
func yamlFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("yaml")
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		out[name] = f.Type
	}
	return out
}

// suggest returns the closest known key within a small edit distance.
func suggest(key string, fields map[string]reflect.Type) string {
	names := make([]string, 0, len(fields))
	for n := range fields {
		names = append(names, n)
	}
	sort.Strings(names)
	best, bestDist := "", 3
	for _, n := range names {
		if d := levenshtein(key, n); d < bestDist {
			best, bestDist = n, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
