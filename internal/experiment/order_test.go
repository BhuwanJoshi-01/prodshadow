package experiment

import (
	"reflect"
	"testing"
	"time"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/migrate"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/workload"
)

func TestScheduleAlternatesOrder(t *testing.T) {
	got := Schedule(3)
	want := []Step{{Baseline, 0}, {Candidate, 0}, {Candidate, 1}, {Baseline, 1}, {Baseline, 2}, {Candidate, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if len(Schedule(0)) != 0 {
		t.Fatal("zero repeats should produce no steps")
	}
}

func TestMigrationWindows(t *testing.T) {
	out := &migrate.Outcome{Started: time.Now(), OffsetMS: 5000, DurationMS: 1200}
	pre, during, ok := migrationWindows(out, 45*time.Second)
	if !ok || pre != (workload.Window{From: 0, To: 5 * time.Second}) {
		t.Fatalf("pre = %+v", pre)
	}
	if during.From != 5*time.Second || during.To != 6250*time.Millisecond {
		t.Fatalf("during = %+v", during)
	}

	long := &migrate.Outcome{Started: time.Now(), OffsetMS: 5000, DurationMS: 90000}
	_, during, _ = migrationWindows(long, 45*time.Second)
	if during.To != 45*time.Second {
		t.Fatalf("window must be clipped to the measurement: %+v", during)
	}

	if _, _, ok := migrationWindows(nil, time.Second); ok {
		t.Fatal("no migration means no windows")
	}
}
