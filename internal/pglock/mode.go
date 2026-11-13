// Package pglock models PostgreSQL table-level lock modes and their conflict
// matrix, as documented in "Explicit Locking":
// https://www.postgresql.org/docs/current/explicit-locking.html
//
// The model is used both to explain static findings ("this statement takes
// ACCESS EXCLUSIVE, which blocks reads") and to interpret observed lock waits.
package pglock

import (
	"fmt"
	"strings"
)

// Mode is a table-level lock mode, ordered from weakest to strongest.
type Mode int

const (
	// None means no table-level lock is taken.
	None Mode = iota
	AccessShare
	RowShare
	RowExclusive
	ShareUpdateExclusive
	Share
	ShareRowExclusive
	Exclusive
	AccessExclusive
)

var modeNames = [...]string{
	None:                 "NONE",
	AccessShare:          "ACCESS SHARE",
	RowShare:             "ROW SHARE",
	RowExclusive:         "ROW EXCLUSIVE",
	ShareUpdateExclusive: "SHARE UPDATE EXCLUSIVE",
	Share:                "SHARE",
	ShareRowExclusive:    "SHARE ROW EXCLUSIVE",
	Exclusive:            "EXCLUSIVE",
	AccessExclusive:      "ACCESS EXCLUSIVE",
}

// pgLocksNames are the spellings used in the pg_locks.mode column.
var pgLocksNames = [...]string{
	None:                 "",
	AccessShare:          "AccessShareLock",
	RowShare:             "RowShareLock",
	RowExclusive:         "RowExclusiveLock",
	ShareUpdateExclusive: "ShareUpdateExclusiveLock",
	Share:                "ShareLock",
	ShareRowExclusive:    "ShareRowExclusiveLock",
	Exclusive:            "ExclusiveLock",
	AccessExclusive:      "AccessExclusiveLock",
}

// String returns the SQL spelling, e.g. "ACCESS EXCLUSIVE".
func (m Mode) String() string {
	if m < None || int(m) >= len(modeNames) {
		return fmt.Sprintf("Mode(%d)", int(m))
	}
	return modeNames[m]
}

// PgLocksName returns the pg_locks.mode spelling, e.g. "AccessExclusiveLock".
func (m Mode) PgLocksName() string {
	if m < None || int(m) >= len(pgLocksNames) {
		return ""
	}
	return pgLocksNames[m]
}

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool { return m >= None && int(m) < len(modeNames) }

// MarshalText implements encoding.TextMarshaler for JSON reports.
func (m Mode) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *Mode) UnmarshalText(b []byte) error {
	parsed, err := Parse(string(b))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// Parse accepts either the SQL spelling ("SHARE ROW EXCLUSIVE") or the
// pg_locks spelling ("ShareRowExclusiveLock"), case-insensitively.
func Parse(s string) (Mode, error) {
	norm := strings.ToUpper(strings.TrimSpace(s))
	for m := None; int(m) < len(modeNames); m++ {
		if norm == modeNames[m] || norm == strings.ToUpper(pgLocksNames[m]) {
			return m, nil
		}
	}
	return None, fmt.Errorf("unknown lock mode %q", s)
}
