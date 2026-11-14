package sqlinspect

import (
	"fmt"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"
)

func init() {
	register(Rule{
		ID:       "PS134",
		Name:     "unbalanced-transaction-control",
		Summary:  "A file opens a transaction it never ends, nests BEGIN, or ends a transaction it did not open",
		Severity: evidence.Warning,
		DocsURL:  docTransactions,
		checkAll: checkTransactionBalance,
	})
}

// checkTransactionBalance follows BEGIN, COMMIT and ROLLBACK through each
// file. PostgreSQL only warns about a nested BEGIN or a COMMIT outside a
// transaction, and silently rolls back a transaction that is still open
// when the session ends, so these mistakes do not fail loudly.
func checkTransactionBalance(c *Context) []Finding {
	var out []Finding
	for _, file := range byFile(c.Statements) {
		var open *Statement
		for _, s := range file {
			if s.Kind != KindTransaction {
				continue
			}
			switch s.Tag {
			case "BEGIN":
				if open != nil {
					f := at(s, fmt.Sprintf("BEGIN inside the transaction opened by statement %s: PostgreSQL only warns and keeps the outer transaction, so the first COMMIT ends both.", open.ID))
					f.Evidence, f.Confidence = evidence.Static, evidence.High
					f.Remediation = "Remove the inner BEGIN, or use SAVEPOINT for a nested unit of work."
					out = append(out, f)
					continue
				}
				open = s
			case "COMMIT", "ROLLBACK":
				if open == nil {
					f := at(s, fmt.Sprintf("%s without a BEGIN earlier in this file. Run directly, the statements before it were not in a transaction; run by a tool that wraps the file in its own transaction, it ends that transaction early.", s.Tag))
					f.Evidence, f.Confidence = evidence.Static, evidence.High
					f.Remediation = "Pair it with a BEGIN in the same file, or remove it and let the migration tool manage the transaction."
					out = append(out, f)
					continue
				}
				open = nil
			}
		}
		if open != nil {
			f := at(open, fmt.Sprintf("The transaction opened here is never committed in %s. When the session ends, PostgreSQL rolls it back and the migration's changes are lost; if the tool runs further files on the same session, they join this transaction.", open.File))
			f.Evidence, f.Confidence = evidence.Static, evidence.High
			f.Remediation = "End the file with COMMIT."
			out = append(out, f)
		}
	}
	return out
}
