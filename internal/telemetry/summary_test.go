package telemetry

import (
	"reflect"
	"testing"
	"time"
)

func TestSummarizeLockQueue(t *testing.T) {
	t0 := time.Unix(1000, 0)
	interval := 100 * time.Millisecond
	samples := []Sample{
		{At: t0.Add(-50 * time.Millisecond), Sessions: []Session{{PID: 1, Application: "prodshadow/worker/0", State: "active"}}},
		{At: t0.Add(1 * time.Second), Sessions: []Session{
			{PID: 9, Application: "prodshadow/background/report/0", State: "idle in transaction", XactMS: 3000},
			{PID: 7, Application: AppMigration, WaitType: "Lock", WaitMS: 400, LockMode: "AccessExclusiveLock", LockType: "relation", Relation: "orders", BlockedBy: []int32{9}},
			{PID: 1, Application: "prodshadow/worker/0", WaitType: "Lock", WaitMS: 350, LockMode: "RowExclusiveLock", Relation: "orders", BlockedBy: []int32{7}},
			{PID: 2, Application: "prodshadow/worker/1", WaitType: "Lock", WaitMS: 300, LockMode: "AccessShareLock", Relation: "orders", BlockedBy: []int32{7}},
		}},
		{At: t0.Add(1100 * time.Millisecond), Sessions: []Session{
			{PID: 9, Application: "prodshadow/background/report/0", State: "idle in transaction", XactMS: 3100},
			{PID: 7, Application: AppMigration, WaitType: "Lock", WaitMS: 500, LockMode: "AccessExclusiveLock", Relation: "orders", BlockedBy: []int32{9, 42}},
			{PID: 1, Application: "prodshadow/worker/0", WaitType: "Lock", WaitMS: 450, LockMode: "RowExclusiveLock", Relation: "orders", BlockedBy: []int32{7}},
		}},
	}
	s := Summarize(samples, 1, interval, t0, time.Second)
	if s.Samples != 3 || s.FailedSamples != 1 || s.SampleIntervalMS != 100 {
		t.Fatalf("counts wrong: %+v", s)
	}
	if s.MaxWaitingSessions != 2 || s.MaxBlockedByMigration != 2 || s.MaxLockWaitMS != 450 {
		t.Fatalf("waiting stats wrong: %+v", s)
	}
	if s.MigrationMaxWaitMS != 500 || !reflect.DeepEqual(s.MigrationBlockedBy, []string{"pid 42", "prodshadow/background/report/0"}) {
		t.Fatalf("migration stats wrong: %+v", s)
	}
	if s.MaxXactMS != 3100 || s.MaxXactApp != "prodshadow/background/report/0" || s.MaxGapMS != 1050 {
		t.Fatalf("xact/gap wrong: %+v", s)
	}
	if s.WaitingSessionSeconds != 0.3 {
		t.Fatalf("waiting session seconds = %v", s.WaitingSessionSeconds)
	}
	if len(s.Timeline) != 1 || s.Timeline[0].OffsetSec != 1 || s.Timeline[0].Waiting != 2 {
		t.Fatalf("timeline wrong: %+v", s.Timeline)
	}
	if s.Relations[0].Relation != "orders" || s.Relations[0].Mode != "AccessExclusiveLock" {
		t.Fatalf("relations wrong: %+v", s.Relations)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	s := Summarize(nil, 0, time.Second, time.Now(), time.Second)
	if s.Samples != 0 || s.Timeline != nil || s.MaxWaitingSessions != 0 {
		t.Fatalf("unexpected summary %+v", s)
	}
}
