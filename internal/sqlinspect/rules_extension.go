package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/evidence"
)

func init() {
	register(Rule{
		ID:       "PS127",
		Name:     "extension-requires-preload",
		Summary:  "The extension only works when the server preloads its library, which needs a restart",
		Severity: evidence.Warning,
		DocsURL:  "https://www.postgresql.org/docs/current/runtime-config-client.html#GUC-SHARED-PRELOAD-LIBRARIES",
		check:    checkExtensionPreload,
	})
}

// preloadExtensions lists widely used extensions whose library must be in
// shared_preload_libraries. CREATE EXTENSION fails for some of them and
// succeeds for others, which then fail or do nothing at run time.
var preloadExtensions = map[string]string{
	"citus":              "CREATE EXTENSION fails",
	"pg_cron":            "CREATE EXTENSION fails",
	"pg_squeeze":         "CREATE EXTENSION fails",
	"pglogical":          "CREATE EXTENSION fails",
	"timescaledb":        "CREATE EXTENSION fails",
	"pg_stat_statements": "its views raise an error",
	"pg_stat_kcache":     "its views raise an error",
	"pg_wait_sampling":   "its views raise an error",
	"pg_qualstats":       "it collects nothing",
	"pgaudit":            "it logs nothing",
}

func checkExtensionPreload(_ *Context, s *Statement) []Finding {
	if s.Kind != KindCreateExtension || len(s.Subcommands) == 0 {
		return nil
	}
	name := s.Subcommands[0]
	effect, ok := preloadExtensions[name]
	if !ok {
		return nil
	}
	f := at(s, fmt.Sprintf("%s needs its library in shared_preload_libraries, which only takes effect after a server restart; where it is not preloaded, %s. Development images often preload it when production does not.", name, effect))
	f.Evidence, f.Confidence = evidence.Heuristic, evidence.Medium
	f.Remediation = fmt.Sprintf("Confirm that every target server lists %s in shared_preload_libraries (on managed services, in the parameter group) before deploying the migration.", name)
	return []Finding{f}
}
