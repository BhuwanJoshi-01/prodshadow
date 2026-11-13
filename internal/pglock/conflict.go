package pglock

// conflicts is the table-level conflict matrix from the PostgreSQL manual
// ("Conflicting Lock Modes"). conflicts[a] lists the modes that a conflicts
// with; the relation is symmetric.
var conflicts = map[Mode][]Mode{
	AccessShare:          {AccessExclusive},
	RowShare:             {Exclusive, AccessExclusive},
	RowExclusive:         {Share, ShareRowExclusive, Exclusive, AccessExclusive},
	ShareUpdateExclusive: {ShareUpdateExclusive, Share, ShareRowExclusive, Exclusive, AccessExclusive},
	Share:                {RowExclusive, ShareUpdateExclusive, ShareRowExclusive, Exclusive, AccessExclusive},
	ShareRowExclusive:    {RowExclusive, ShareUpdateExclusive, Share, ShareRowExclusive, Exclusive, AccessExclusive},
	Exclusive:            {RowShare, RowExclusive, ShareUpdateExclusive, Share, ShareRowExclusive, Exclusive, AccessExclusive},
	AccessExclusive:      {AccessShare, RowShare, RowExclusive, ShareUpdateExclusive, Share, ShareRowExclusive, Exclusive, AccessExclusive},
}

// Conflicts reports whether two modes conflict on the same relation.
func Conflicts(a, b Mode) bool {
	for _, c := range conflicts[a] {
		if c == b {
			return true
		}
	}
	return false
}

// BlocksReads reports whether holding (or queueing for) m blocks plain
// SELECT statements, which take ACCESS SHARE.
func (m Mode) BlocksReads() bool { return Conflicts(m, AccessShare) }

// BlocksWrites reports whether m blocks INSERT/UPDATE/DELETE, which take
// ROW EXCLUSIVE.
func (m Mode) BlocksWrites() bool { return Conflicts(m, RowExclusive) }

// BlocksRowLocking reports whether m blocks SELECT ... FOR UPDATE/SHARE,
// which take ROW SHARE.
func (m Mode) BlocksRowLocking() bool { return Conflicts(m, RowShare) }

// Impact summarises what traffic a lock mode stops.
type Impact string

const (
	ImpactNone       Impact = "none"
	ImpactDDLOnly    Impact = "blocks-concurrent-ddl"
	ImpactWrites     Impact = "blocks-writes"
	ImpactAllTraffic Impact = "blocks-reads-and-writes"
)

// Impact classifies m from the point of view of application traffic.
func (m Mode) Impact() Impact {
	switch {
	case m == None:
		return ImpactNone
	case m.BlocksReads():
		return ImpactAllTraffic
	case m.BlocksWrites():
		return ImpactWrites
	case Conflicts(m, ShareUpdateExclusive) || Conflicts(m, m):
		return ImpactDDLOnly
	default:
		return ImpactNone
	}
}

// Strongest returns the strongest of the given modes.
func Strongest(modes ...Mode) Mode {
	best := None
	for _, m := range modes {
		if m > best {
			best = m
		}
	}
	return best
}
