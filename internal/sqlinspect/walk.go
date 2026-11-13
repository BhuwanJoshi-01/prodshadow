package sqlinspect

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// walk visits every message in a parse tree depth-first. Returning false
// from visit stops descent into that message's children.
func walk(msg proto.Message, visit func(protoreflect.Message) bool) {
	if msg == nil {
		return
	}
	walkReflect(msg.ProtoReflect(), visit)
}

func walkReflect(m protoreflect.Message, visit func(protoreflect.Message) bool) {
	if !m.IsValid() || !visit(m) {
		return
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList() && fd.Message() != nil:
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				walkReflect(list.Get(i).Message(), visit)
			}
		case fd.IsMap():
			// The PostgreSQL parse tree has no map fields.
		case fd.Message() != nil:
			walkReflect(v.Message(), visit)
		}
		return true
	})
}

// funcCalls returns the lower-cased names of all functions called in expr.
func funcCalls(expr *pg_query.Node) []string {
	var names []string
	walk(expr, func(m protoreflect.Message) bool {
		if fc, ok := m.Interface().(*pg_query.FuncCall); ok {
			names = append(names, qualifiedName(fc.Funcname))
		}
		return true
	})
	return names
}

// qualifiedName joins a list of String nodes with dots, lower-cased.
func qualifiedName(parts []*pg_query.Node) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := p.GetString_(); s != nil {
			out = append(out, strings.ToLower(s.Sval))
		}
	}
	return strings.Join(out, ".")
}

// lastName returns the unqualified part of a qualified name.
func lastName(qualified string) string {
	if i := strings.LastIndexByte(qualified, '.'); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// relName renders a RangeVar as [schema.]relation.
func relName(rv *pg_query.RangeVar) string {
	if rv == nil {
		return ""
	}
	if rv.Schemaname != "" {
		return rv.Schemaname + "." + rv.Relname
	}
	return rv.Relname
}

// typeName returns the unqualified, lower-cased name of a type.
func typeName(tn *pg_query.TypeName) string {
	if tn == nil {
		return ""
	}
	return lastName(qualifiedName(tn.Names))
}
