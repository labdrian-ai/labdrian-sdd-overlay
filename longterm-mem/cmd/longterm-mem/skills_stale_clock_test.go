package main

import (
	"strings"
	"testing"
	"time"
)

// `skills-stale` judges whether a skill is quiet by the clock the commands are given, and not by the machine's:
// the same record is quiet or not according to the instant the clock tells. The goldens cannot say this (they
// are recorded with records so old or so new that no clock changes them), so these cases put the clock on either
// side of the half year.
func TestSkillsStaleJudgesQuietnessByTheClockItIsGiven(t *testing.T) {
	at := func(year int, month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	for _, tc := range []struct {
		name         string
		lastObserved string
		now          time.Time
		quiet        bool
	}{
		{"a month after the last observation", "2026-07-01T00:00:00Z", at(2026, 8, 1), false},
		{"a day short of the half year", "2026-07-01T00:00:00Z", at(2026, 12, 27), false},
		{"a day past the half year", "2026-07-01T00:00:00Z", at(2026, 12, 29), true},
		{"a year after the last observation", "2026-07-01T00:00:00Z", at(2027, 7, 1), true},
		{"before the last observation", "2000-01-01T00:00:00Z", at(1999, 1, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newSkillsStaleWorld(t, false)
			w.lock(t, lockOf("clocked"))
			w.skillFiles(t, "clocked", skillText("Nothing to run.\n"))
			w.observed(t, "clocked", tc.lastObserved)
			w.commands.clock = fixedClock{at: tc.now}

			code, stdout, stderr := w.run(t, []string{"skills-stale", "--project-root", w.root, "--project", skillsStaleProject})

			if code != exitOK || stderr != "" {
				t.Fatalf("skills-stale = exit %d, stderr %q; want a report", code, stderr)
			}
			if got := strings.Contains(stdout, "QUIET since "+tc.lastObserved); got != tc.quiet {
				t.Errorf("QUIET reported = %v with the clock at %s, want %v:\n%s", got, tc.now.Format(time.DateOnly), tc.quiet, stdout)
			}
		})
	}
}
