package telemetry

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Application name prefixes used by ProdShadow sessions.
const (
	AppMigration  = "prodshadow/migration"
	AppWorker     = "prodshadow/worker/"
	AppBackground = "prodshadow/background/"
)

// Summary condenses samples for one phase.
type Summary struct {
	SampleIntervalMS int     `json:"sample_interval_ms"`
	Samples          int     `json:"samples"`
	FailedSamples    int     `json:"failed_samples"`
	MaxGapMS         float64 `json:"max_gap_ms"`
	// Workload sessions (workers and background) waiting on locks.
	MaxWaitingSessions    int     `json:"max_waiting_sessions"`
	MaxLockWaitMS         float64 `json:"max_lock_wait_ms"`
	WaitingSessionSeconds float64 `json:"waiting_session_seconds"`
	// The migration session.
	MigrationMaxWaitMS    float64  `json:"migration_max_wait_ms"`
	MigrationBlockedBy    []string `json:"migration_blocked_by,omitempty"`
	MaxBlockedByMigration int      `json:"max_blocked_by_migration"`
	// Longest open transaction of any observed session.
	MaxXactMS float64        `json:"max_transaction_ms"`
	Relations []RelationWait `json:"relations,omitempty"`
	Timeline  []Point        `json:"timeline,omitempty"`
}

// RelationWait aggregates lock waits on one relation.
type RelationWait struct {
	Relation   string  `json:"relation"`
	Mode       string  `json:"mode"`
	MaxWaiters int     `json:"max_waiters"`
	MaxWaitMS  float64 `json:"max_wait_ms"`
}

// Point is the peak lock-wait state within one timeline bucket.
type Point struct {
	OffsetSec float64 `json:"offset_sec"`
	Waiting   int     `json:"waiting"`
	MaxWaitMS float64 `json:"max_wait_ms"`
}

func isWorkload(app string) bool {
	return strings.HasPrefix(app, AppWorker) || strings.HasPrefix(app, AppBackground)
}

// Summarize aggregates samples taken during a phase. Offsets in the
// timeline are relative to measureStart and bucketed by width.
func Summarize(samples []Sample, failed int, interval time.Duration, measureStart time.Time, width time.Duration) Summary {
	s := Summary{SampleIntervalMS: int(interval / time.Millisecond), Samples: len(samples), FailedSamples: failed}
	if width <= 0 {
		width = time.Second
	}
	relations := map[[2]string]*RelationWait{}
	blockers := map[string]bool{}
	points := map[int]*Point{}

	var prev time.Time
	for _, smp := range samples {
		if !prev.IsZero() {
			s.MaxGapMS = math.Max(s.MaxGapMS, float64(smp.At.Sub(prev))/float64(time.Millisecond))
		}
		prev = smp.At
		appByPID := map[int32]string{}
		var migrationPID int32 = -1
		for _, x := range smp.Sessions {
			appByPID[x.PID] = x.Application
			if x.Application == AppMigration {
				migrationPID = x.PID
			}
		}
		waiting, blockedByMigration := 0, 0
		perRel := map[[2]string]int{}
		sampleMax := 0.0
		for _, x := range smp.Sessions {
			s.MaxXactMS = math.Max(s.MaxXactMS, x.XactMS)
			if !x.WaitingOnLock() {
				continue
			}
			key := [2]string{x.Relation, x.LockMode}
			if x.Relation == "" {
				key[0] = x.LockType
			}
			perRel[key]++
			rw := relations[key]
			if rw == nil {
				rw = &RelationWait{Relation: key[0], Mode: x.LockMode}
				relations[key] = rw
			}
			rw.MaxWaitMS = math.Max(rw.MaxWaitMS, x.WaitMS)

			switch {
			case x.Application == AppMigration:
				s.MigrationMaxWaitMS = math.Max(s.MigrationMaxWaitMS, x.WaitMS)
				for _, b := range x.BlockedBy {
					if app, ok := appByPID[b]; ok && app != "" {
						blockers[app] = true
					} else {
						blockers["pid "+strconv.Itoa(int(b))] = true
					}
				}
			case isWorkload(x.Application):
				waiting++
				s.MaxLockWaitMS = math.Max(s.MaxLockWaitMS, x.WaitMS)
				sampleMax = math.Max(sampleMax, x.WaitMS)
				for _, b := range x.BlockedBy {
					if b == migrationPID {
						blockedByMigration++
						break
					}
				}
			}
		}
		for key, n := range perRel {
			if n > relations[key].MaxWaiters {
				relations[key].MaxWaiters = n
			}
		}
		s.MaxWaitingSessions = max(s.MaxWaitingSessions, waiting)
		s.MaxBlockedByMigration = max(s.MaxBlockedByMigration, blockedByMigration)
		s.WaitingSessionSeconds += float64(waiting) * interval.Seconds()

		off := smp.At.Sub(measureStart)
		if off >= 0 {
			b := int(off / width)
			p := points[b]
			if p == nil {
				p = &Point{OffsetSec: (time.Duration(b) * width).Seconds()}
				points[b] = p
			}
			p.Waiting = max(p.Waiting, waiting)
			p.MaxWaitMS = math.Max(p.MaxWaitMS, sampleMax)
		}
	}

	for _, rw := range relations {
		rw.MaxWaitMS = round1(rw.MaxWaitMS)
		s.Relations = append(s.Relations, *rw)
	}
	sort.Slice(s.Relations, func(i, j int) bool {
		if s.Relations[i].MaxWaitMS != s.Relations[j].MaxWaitMS {
			return s.Relations[i].MaxWaitMS > s.Relations[j].MaxWaitMS
		}
		return s.Relations[i].Relation < s.Relations[j].Relation
	})
	for app := range blockers {
		s.MigrationBlockedBy = append(s.MigrationBlockedBy, app)
	}
	sort.Strings(s.MigrationBlockedBy)
	keys := make([]int, 0, len(points))
	for k := range points {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		p := *points[k]
		p.MaxWaitMS = round1(p.MaxWaitMS)
		s.Timeline = append(s.Timeline, p)
	}
	s.MaxGapMS = round1(s.MaxGapMS)
	s.MaxLockWaitMS = round1(s.MaxLockWaitMS)
	s.MigrationMaxWaitMS = round1(s.MigrationMaxWaitMS)
	s.MaxXactMS = round1(s.MaxXactMS)
	s.WaitingSessionSeconds = round1(s.WaitingSessionSeconds)
	return s
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
