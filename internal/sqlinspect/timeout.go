package sqlinspect

import (
	"strconv"
	"strings"
	"time"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// settingValue returns the setting name and its value as written in a SET
// statement, e.g. ("lock_timeout", "2s"). ok is false for other statements
// and for RESET, SET ... TO DEFAULT and SET ... FROM CURRENT.
func settingValue(s *Statement) (name, value string, ok bool) {
	if s.Kind != KindSet || s.node == nil {
		return "", "", false
	}
	vs := s.node.GetVariableSetStmt()
	if vs == nil || vs.Kind != pg_query.VariableSetKind_VAR_SET_VALUE || len(vs.Args) != 1 {
		return "", "", false
	}
	c := vs.Args[0].GetAConst()
	if c == nil {
		return "", "", false
	}
	switch {
	case c.GetSval() != nil:
		return strings.ToLower(vs.Name), c.GetSval().Sval, true
	case c.GetIval() != nil:
		return strings.ToLower(vs.Name), strconv.Itoa(int(c.GetIval().Ival)), true
	}
	return "", "", false
}

// parseTimeout interprets a PostgreSQL time setting. A bare number is in
// milliseconds; units ms, s, min, h and d are accepted.
func parseTimeout(v string) (time.Duration, bool) {
	v = strings.TrimSpace(strings.ToLower(v))
	units := []struct {
		suffix string
		unit   time.Duration
	}{{"ms", time.Millisecond}, {"min", time.Minute}, {"s", time.Second}, {"h", time.Hour}, {"d", 24 * time.Hour}}
	for _, u := range units {
		if num, found := strings.CutSuffix(v, u.suffix); found {
			f, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
			if err != nil || f < 0 {
				return 0, false
			}
			return time.Duration(f * float64(u.unit)), true
		}
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return time.Duration(f * float64(time.Millisecond)), true
}
