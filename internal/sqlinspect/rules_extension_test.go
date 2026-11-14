package sqlinspect

import "testing"

func TestPS127ExtensionRequiresPreload(t *testing.T) {
	expectCount(t, findings(t, "PS127", Options{}, "CREATE EXTENSION pg_cron;"), 1)
	expectCount(t, findings(t, "PS127", Options{}, "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;"), 1)
	expectCount(t, findings(t, "PS127", Options{}, "CREATE EXTENSION TimescaleDB;"), 1)
	expectCount(t, findings(t, "PS127", Options{}, "CREATE EXTENSION pg_trgm;"), 0)
	expectCount(t, findings(t, "PS127", Options{}, "CREATE EXTENSION pgcrypto;"), 0)
}
