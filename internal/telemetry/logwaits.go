package telemetry

import (
	"bufio"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LogWait is one lock-wait message written by the server because
// log_lock_waits is on. Unlike sampling, the log records every wait that
// lasts longer than deadlock_timeout, with its exact duration.
type LogWait struct {
	At       time.Time `json:"at"`
	PID      int32     `json:"pid"`
	Mode     string    `json:"mode"`
	Object   string    `json:"object"`
	Acquired bool      `json:"acquired"`
	AfterMS  float64   `json:"after_ms"`
}

// logWaitRe matches lines such as
//
//	2026-11-13 15:08:12.739 UTC [79] LOG:  process 79 acquired AccessExclusiveLock on relation 16384 of database 5 after 1196.720 ms
//
// It is not anchored, because the image's entrypoint can prefix lines with
// its own output.
var logWaitRe = regexp.MustCompile(`(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}) (\S+) \[\d+\] LOG:  process (\d+) (still waiting for|acquired) (\w+) on (.+?) after ([\d.]+) ms`)

// ParseLockWaits extracts lock-wait messages logged in [from, to).
func ParseLockWaits(logs string, from, to time.Time) []LogWait {
	var out []LogWait
	sc := bufio.NewScanner(strings.NewReader(logs))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		m := logWaitRe.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		at, err := time.Parse("2006-01-02 15:04:05.000 MST", m[1]+" "+m[2])
		if err != nil {
			continue
		}
		if at.Before(from) || !at.Before(to) {
			continue
		}
		pid, _ := strconv.ParseInt(m[3], 10, 32)
		after, _ := strconv.ParseFloat(m[7], 64)
		out = append(out, LogWait{At: at, PID: int32(pid), Acquired: m[4] == "acquired", Mode: m[5], Object: m[6], AfterMS: after})
	}
	return out
}

// LogSummary condenses logged waits for one phase.
type LogSummary struct {
	Events                int     `json:"events"`
	MaxWorkloadWaitMS     float64 `json:"max_workload_wait_ms"`
	MaxMigrationWaitMS    float64 `json:"max_migration_wait_ms"`
	WorkloadSessionsWaits int     `json:"workload_sessions_waited"`
}

// SummarizeLogWaits attributes logged waits to sessions using the
// application names seen by the sampler.
func SummarizeLogWaits(waits []LogWait, apps map[int32]string) LogSummary {
	s := LogSummary{Events: len(waits)}
	waited := map[int32]bool{}
	for _, w := range waits {
		app := apps[w.PID]
		switch {
		case app == AppMigration:
			s.MaxMigrationWaitMS = math.Max(s.MaxMigrationWaitMS, w.AfterMS)
		case isWorkload(app):
			s.MaxWorkloadWaitMS = math.Max(s.MaxWorkloadWaitMS, w.AfterMS)
			waited[w.PID] = true
		}
	}
	s.WorkloadSessionsWaits = len(waited)
	s.MaxWorkloadWaitMS = round1(s.MaxWorkloadWaitMS)
	s.MaxMigrationWaitMS = round1(s.MaxMigrationWaitMS)
	return s
}

// ApplicationsByPID maps backend PIDs to application names across samples.
func ApplicationsByPID(samples []Sample) map[int32]string {
	out := map[int32]string{}
	for _, smp := range samples {
		for _, x := range smp.Sessions {
			if x.Application != "" {
				out[x.PID] = x.Application
			}
		}
	}
	return out
}
