package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/internal/workload"
)

// TestExamplePlansPredictConflicts pins the conflicts plan predicts for
// every example, without a container engine. The integration tests check
// that the measured behaviour matches.
func TestExamplePlansPredictConflicts(t *testing.T) {
	cases := []struct {
		example   string
		migration string
		conflicts []string // session/table, background sessions marked (bg)
		unblocked []string
	}{
		{example: "safe-add-column", conflicts: []string{"change_plan/users", "get_user/users"}},
		{example: "long-running-transaction", conflicts: []string{"list_invoices/invoices", "pay_invoice/invoices", "revenue_report (bg)/invoices"}},
		{example: "blocking-index", conflicts: []string{"create_order/orders", "update_status/orders"}, unblocked: []string{"get_order"}},
		// Row locks, not table locks: nothing conflicts, yet writers stall.
		{example: "backfill-heavy", unblocked: []string{"get_subscriber", "toggle_opt_in"}},
		{example: "rolling-deploy", conflicts: []string{"v1_get_product/products", "v2_get_product/products"}},
		{example: "partition-maintenance", conflicts: []string{"nightly_export (bg)/events", "recent_events/events", "record_event/events"}},
		{example: "partition-maintenance", migration: "alternatives/0031_events_2027.sql", unblocked: []string{"recent_events", "record_event", "nightly_export"}},
		{example: "matview-refresh", conflicts: []string{"customer_revenue/daily_revenue"}, unblocked: []string{"record_order"}},
		{example: "foreign-key-drop", conflicts: []string{"customer_export (bg)/customers", "customer_invoices/invoices", "customer_profile/customers", "issue_invoice/customers", "issue_invoice/invoices"}},
	}
	for _, c := range cases {
		t.Run(c.example+"/"+c.migration, func(t *testing.T) {
			dir := filepath.Join("..", "..", "examples", c.example)
			args := []string{"plan", "--json", "--config", "prodshadow.yaml"}
			if c.migration != "" {
				args = append(args, "--migration", c.migration)
			}
			r := run(t, dir, nil, args...)
			var out struct {
				Conflicts []workload.Conflict `json:"conflicts"`
				Unblocked []string            `json:"unblocked"`
			}
			if r.code != ExitOK || json.Unmarshal([]byte(r.stdout), &out) != nil {
				t.Fatalf("plan: %+v", r)
			}
			var got []string
			for _, cf := range out.Conflicts {
				s := cf.Session
				if cf.Background {
					s += " (bg)"
				}
				if k := s + "/" + cf.Table; !slices.Contains(got, k) {
					got = append(got, k)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, c.conflicts) {
				t.Errorf("conflicts = %v, want %v", got, c.conflicts)
			}
			if !slices.Equal(out.Unblocked, c.unblocked) {
				t.Errorf("unblocked = %v, want %v", out.Unblocked, c.unblocked)
			}
		})
	}
}
