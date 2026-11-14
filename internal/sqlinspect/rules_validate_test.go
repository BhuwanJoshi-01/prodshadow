package sqlinspect

import (
	"strings"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pglock"
)

func TestPS136ValidatedInSameTransaction(t *testing.T) {
	addFK := "ALTER TABLE refunds ADD CONSTRAINT refunds_order_fk FOREIGN KEY (order_id) REFERENCES orders (id) NOT VALID;\n"
	validateFK := "ALTER TABLE refunds VALIDATE CONSTRAINT refunds_order_fk;\n"
	got := findings(t, "PS136", Options{Transaction: TxnModeAuto}, addFK+validateFK)
	expectCount(t, got, 1)
	if got[0].Lock != pglock.ShareRowExclusive || !strings.Contains(got[0].Message, "statement 1.1") {
		t.Errorf("finding: %+v", got[0])
	}
	addCheck := "ALTER TABLE orders ADD CONSTRAINT orders_total_positive CHECK (total >= 0) NOT VALID;\n"
	got = findings(t, "PS136", Options{Transaction: TxnModeAuto}, addCheck+"ALTER TABLE orders VALIDATE CONSTRAINT orders_total_positive;\n")
	expectCount(t, got, 1)
	if got[0].Lock != pglock.AccessExclusive {
		t.Errorf("CHECK constraints are added under ACCESS EXCLUSIVE: %+v", got[0])
	}
	// Separate transactions: the recommended pattern.
	expectCount(t, findings(t, "PS136", Options{Transaction: TxnModePerFile}, addFK, validateFK), 0)
	expectCount(t, findings(t, "PS136", Options{Transaction: TxnModeNone}, addFK+validateFK), 0)
	// Explicit COMMIT between them.
	expectCount(t, findings(t, "PS136", Options{}, "BEGIN;\n"+addFK+"COMMIT;\nBEGIN;\n"+validateFK+"COMMIT;\n"), 0)
	// Validating a constraint added by an earlier migration.
	expectCount(t, findings(t, "PS136", Options{}, validateFK), 0)
}

func TestPS137ConstraintLeftNotValid(t *testing.T) {
	add := "ALTER TABLE refunds ADD CONSTRAINT refunds_order_fk FOREIGN KEY (order_id) REFERENCES orders (id) NOT VALID;\n"
	expectCount(t, findings(t, "PS137", Options{}, add), 1)
	expectCount(t, findings(t, "PS137", Options{}, add, "ALTER TABLE refunds VALIDATE CONSTRAINT refunds_order_fk;\n"), 0)
	expectCount(t, findings(t, "PS137", Options{}, add+"ALTER TABLE refunds VALIDATE CONSTRAINT other_fk;\n"), 1)
	expectCount(t, findings(t, "PS137", Options{}, "ALTER TABLE orders ADD CONSTRAINT orders_total_positive CHECK (total >= 0);\n"), 0)
}
