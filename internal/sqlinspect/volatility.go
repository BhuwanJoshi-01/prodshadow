package sqlinspect

import (
	"slices"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// volatileFunctions are commonly used volatile functions. A column default
// that calls one of them forces PostgreSQL to rewrite the table, because the
// value must be computed for every existing row.
var volatileFunctions = []string{
	"random", "gen_random_uuid", "uuidv4", "uuidv7",
	"uuid_generate_v1", "uuid_generate_v1mc", "uuid_generate_v4",
	"clock_timestamp", "timeofday", "nextval", "setseed", "txid_current",
	"pg_current_xact_id",
}

// stableFunctions are common non-volatile functions. Defaults using only
// these are evaluated once and stored as metadata (PostgreSQL 11+).
var stableFunctions = []string{
	"now", "transaction_timestamp", "statement_timestamp", "current_date",
	"lower", "upper", "length", "abs", "coalesce", "concat", "md5",
	"to_char", "to_timestamp", "to_date", "make_interval", "jsonb_build_object",
	"json_build_object", "jsonb_build_array", "array_length", "date_trunc",
	"timezone", "replace", "substring", "trim", "round", "floor", "ceil",
}

var serialTypes = []string{"serial", "serial2", "serial4", "serial8", "smallserial", "bigserial"}

// Volatility classifies a default expression.
type Volatility int

const (
	// NotVolatile defaults are stored as metadata without a rewrite.
	NotVolatile Volatility = iota
	// MaybeVolatile defaults call functions this analyser does not know.
	MaybeVolatile
	// Volatile defaults force a table rewrite.
	Volatile
)

// expressionVolatility inspects function calls in expr.
func expressionVolatility(expr *pg_query.Node) (Volatility, []string) {
	var unknown []string
	result := NotVolatile
	for _, fn := range funcCalls(expr) {
		name := lastName(fn)
		switch {
		case slices.Contains(volatileFunctions, name):
			return Volatile, []string{fn}
		case slices.Contains(stableFunctions, name):
		default:
			unknown = append(unknown, fn)
			result = MaybeVolatile
		}
	}
	return result, unknown
}

// columnRewrite explains why adding col rewrites the table, or returns "".
// The second result reports whether the conclusion is certain.
func columnRewrite(col *pg_query.ColumnDef) (string, bool) {
	if col == nil {
		return "", true
	}
	if slices.Contains(serialTypes, typeName(col.TypeName)) {
		return "a serial column fills every existing row from a sequence", true
	}
	if col.Identity != "" {
		return "an identity column fills every existing row from a sequence", true
	}
	if col.Generated != "" {
		return "a stored generated column is computed for every existing row", true
	}
	for _, c := range col.Constraints {
		con := c.GetConstraint()
		if con == nil {
			continue
		}
		switch con.Contype {
		case pg_query.ConstrType_CONSTR_IDENTITY:
			return "an identity column fills every existing row from a sequence", true
		case pg_query.ConstrType_CONSTR_GENERATED:
			return "a stored generated column is computed for every existing row", true
		case pg_query.ConstrType_CONSTR_DEFAULT:
			v, fns := expressionVolatility(con.RawExpr)
			switch v {
			case Volatile:
				return "the default calls volatile function " + fns[0] + "(), so every existing row is rewritten", true
			case MaybeVolatile:
				return "the default calls " + fns[0] + "(), which may be volatile; if it is, every existing row is rewritten", false
			}
		}
	}
	if v, fns := expressionVolatility(col.RawDefault); v != NotVolatile {
		return "the default calls " + fns[0] + "(), which may force a rewrite", v == Volatile
	}
	return "", true
}

// hasConstraint reports whether col declares a constraint of type t.
func hasConstraint(col *pg_query.ColumnDef, t pg_query.ConstrType) bool {
	if col == nil {
		return false
	}
	for _, c := range col.Constraints {
		if con := c.GetConstraint(); con != nil && con.Contype == t {
			return true
		}
	}
	return false
}
