package telemetry

import (
	"testing"
	"time"
)

const sampleLog = `waiting for server to start....2026-11-13 15:08:06.946 UTC [41] LOG:  starting PostgreSQL 17.12
2026-11-13 15:08:11.742 UTC [79] LOG:  process 79 still waiting for AccessExclusiveLock on relation 16384 of database 5 after 200.185 ms
2026-11-13 15:08:11.742 UTC [79] DETAIL:  Process holding the lock: 78. Wait queue: 79.
2026-11-13 15:08:12.100 UTC [80] LOG:  process 80 still waiting for RowExclusiveLock on relation 16384 of database 5 after 200.020 ms
2026-11-13 15:08:12.739 UTC [79] LOG:  process 79 acquired AccessExclusiveLock on relation 16384 of database 5 after 1196.720 ms
2026-11-13 15:08:12.740 UTC [80] LOG:  process 80 acquired RowExclusiveLock on relation 16384 of database 5 after 840.500 ms
2026-11-13 15:09:00.000 UTC [81] LOG:  process 81 acquired ShareLock on transaction 731 after 300.000 ms
`

func TestParseLockWaits(t *testing.T) {
	from := time.Date(2026, 11, 13, 15, 8, 0, 0, time.UTC)
	to := from.Add(time.Minute)
	waits := ParseLockWaits(sampleLog, from, to)
	if len(waits) != 4 {
		t.Fatalf("expected 4 waits inside the window, got %d: %+v", len(waits), waits)
	}
	if !waits[2].Acquired || waits[2].AfterMS != 1196.72 || waits[2].Mode != "AccessExclusiveLock" || waits[2].PID != 79 {
		t.Fatalf("unexpected wait %+v", waits[2])
	}
	s := SummarizeLogWaits(waits, map[int32]string{79: AppMigration, 80: "prodshadow/worker/3"})
	if s.Events != 4 || s.MaxMigrationWaitMS != 1196.7 || s.MaxWorkloadWaitMS != 840.5 || s.WorkloadSessionsWaits != 1 {
		t.Fatalf("unexpected summary %+v", s)
	}
}

func TestApplicationsByPID(t *testing.T) {
	m := ApplicationsByPID([]Sample{{Sessions: []Session{{PID: 1, Application: "a"}}}, {Sessions: []Session{{PID: 2, Application: "b"}, {PID: 3}}}})
	if len(m) != 2 || m[1] != "a" || m[2] != "b" {
		t.Fatalf("got %v", m)
	}
}
