package sqlinspect

// TransactionMode mirrors migration.transaction in the configuration.
type TransactionMode string

const (
	// TxnModeAuto wraps each file in a transaction unless it controls
	// transactions itself or contains a statement that cannot run inside
	// a transaction block.
	TxnModeAuto TransactionMode = "auto"
	// TxnModePerFile always wraps each file in a transaction.
	TxnModePerFile TransactionMode = "per-file"
	// TxnModeNone runs every statement in autocommit mode unless the file
	// opens transactions explicitly.
	TxnModeNone TransactionMode = "none"
)

// Group is a unit of execution: either a single autocommit statement or a
// sequence of statements that share one transaction.
type Group struct {
	Statements []*Statement
	// Wrapped means the executor adds BEGIN/COMMIT around Statements.
	Wrapped bool
	// Explicit means the file itself opened the transaction with BEGIN.
	Explicit bool
}

// InTransaction reports whether the group's statements share a transaction.
func (g Group) InTransaction() bool { return g.Wrapped || g.Explicit }

// Plan splits statements into execution groups, file by file. The
// migration executor and the static rules use the same plan so that a
// finding about transaction scope matches what is actually executed.
func Plan(stmts []*Statement, mode TransactionMode) []Group {
	groups := make([]Group, 0, len(stmts))
	for _, file := range byFile(stmts) {
		groups = append(groups, planFile(file, mode)...)
	}
	return groups
}

func byFile(stmts []*Statement) [][]*Statement {
	var out [][]*Statement
	for i, s := range stmts {
		if i == 0 || stmts[i-1].File != s.File {
			out = append(out, nil)
		}
		out[len(out)-1] = append(out[len(out)-1], s)
	}
	return out
}

func planFile(stmts []*Statement, mode TransactionMode) []Group {
	controls, noBlock := false, false
	for _, s := range stmts {
		controls = controls || s.Txn == TxnControl
		noBlock = noBlock || s.Txn == TxnNotInBlock
	}
	wrap := mode == TxnModePerFile || (mode == TxnModeAuto && !controls && !noBlock)
	if wrap && !controls {
		return []Group{{Statements: stmts, Wrapped: true}}
	}

	// Follow explicit BEGIN/COMMIT; everything else autocommits.
	var groups []Group
	var open *Group
	for _, s := range stmts {
		if s.Kind == KindTransaction {
			switch s.Tag {
			case "BEGIN":
				if open == nil {
					open = &Group{Explicit: true}
				}
				open.Statements = append(open.Statements, s)
				continue
			case "COMMIT", "ROLLBACK":
				if open != nil {
					open.Statements = append(open.Statements, s)
					groups = append(groups, *open)
					open = nil
					continue
				}
			}
		}
		if open != nil {
			open.Statements = append(open.Statements, s)
			continue
		}
		groups = append(groups, Group{Statements: []*Statement{s}})
	}
	if open != nil {
		groups = append(groups, *open)
	}
	return groups
}
