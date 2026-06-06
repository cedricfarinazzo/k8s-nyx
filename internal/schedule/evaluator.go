/*
Copyright 2026.

Licensed under the MIT License.
*/

// Package schedule evaluates a SleepSchedule against wall-clock time in its own
// IANA timezone: whether it is currently Awake or Asleep, and when it next flips.
package schedule

import (
	"fmt"
	"time"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

// scanHorizon bounds how far ahead nextTransition looks. A week covers the
// recurring weekly pattern; the extra day absorbs a DST shift landing on a boundary.
const scanHorizon = 8 * 24 * time.Hour

// Result is the outcome of evaluating a schedule at an instant.
type Result struct {
	// Awake is true when the instant falls inside an awake window.
	Awake bool
	// Phase is the corresponding SleepSchedule phase (Awake or Asleep).
	Phase nyxv1alpha1.SleepSchedulePhase
	// NextTransition is the next instant the phase flips. Zero if none was found
	// within the scan horizon (should not happen for a recurring weekly schedule).
	NextTransition time.Time
}

var weekdayByDay = map[nyxv1alpha1.Day]time.Weekday{
	"Sun": time.Sunday,
	"Mon": time.Monday,
	"Tue": time.Tuesday,
	"Wed": time.Wednesday,
	"Thu": time.Thursday,
	"Fri": time.Friday,
	"Sat": time.Saturday,
}

// Evaluate reports whether the schedule is awake at now and when it next flips.
// now is converted into the schedule's timezone before any comparison, so callers
// may pass any location (including UTC) without affecting the result.
func Evaluate(spec nyxv1alpha1.SleepScheduleSpec, now time.Time) (Result, error) {
	loc, err := time.LoadLocation(spec.Timezone)
	if err != nil {
		return Result{}, fmt.Errorf("load timezone %q: %w", spec.Timezone, err)
	}
	local := now.In(loc)

	awake := isAwakeAt(spec, local)
	res := Result{Awake: awake, Phase: phaseFor(awake)}

	// Find the earliest upcoming boundary whose phase differs from the current one.
	limit := local.Add(scanHorizon)
	for _, b := range candidateBoundaries(spec, loc, local, limit) {
		if isAwakeAt(spec, b) != awake {
			res.NextTransition = b
			break
		}
	}
	return res, nil
}

func phaseFor(awake bool) nyxv1alpha1.SleepSchedulePhase {
	if awake {
		return nyxv1alpha1.PhaseAwake
	}
	return nyxv1alpha1.PhaseAsleep
}

// isAwakeAt reports whether t (already in the schedule's location) is inside any
// awake window. Windows are half-open [from, to) on each of their days.
func isAwakeAt(spec nyxv1alpha1.SleepScheduleSpec, t time.Time) bool {
	minutes := t.Hour()*60 + t.Minute()
	for _, w := range spec.Awake {
		if !windowCoversDay(w, t.Weekday()) {
			continue
		}
		from, okF := parseHHMM(w.From)
		to, okT := parseHHMM(w.To)
		if !okF || !okT {
			continue
		}
		if minutes >= from && minutes < to {
			return true
		}
	}
	return false
}

func windowCoversDay(w nyxv1alpha1.AwakeWindow, wd time.Weekday) bool {
	for _, d := range w.Days {
		if weekdayByDay[d] == wd {
			return true
		}
	}
	return false
}

// candidateBoundaries returns the window start/end instants strictly after `after`
// and at or before `limit`, anchored to wall-clock local time (DST-correct), sorted
// ascending. Each window contributes its from and to on every day in the horizon it
// applies to.
func candidateBoundaries(spec nyxv1alpha1.SleepScheduleSpec, loc *time.Location, after, limit time.Time) []time.Time {
	var out []time.Time
	startDay := time.Date(after.Year(), after.Month(), after.Day(), 0, 0, 0, 0, loc)
	for day := startDay; !day.After(limit); day = day.AddDate(0, 0, 1) {
		for _, w := range spec.Awake {
			if !windowCoversDay(w, day.Weekday()) {
				continue
			}
			for _, hhmm := range []string{w.From, w.To} {
				m, ok := parseHHMM(hhmm)
				if !ok {
					continue
				}
				// time.Date normalises the wall-clock time within loc, so DST shifts
				// resolve to the correct absolute instant.
				b := time.Date(day.Year(), day.Month(), day.Day(), m/60, m%60, 0, 0, loc)
				if b.After(after) && !b.After(limit) {
					out = append(out, b)
				}
			}
		}
	}
	insertionSort(out)
	return out
}

func parseHHMM(s string) (int, bool) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%02d:%02d", &h, &m); err != nil {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// insertionSort sorts boundaries ascending; the slice is small (≤ ~2 per window per day).
func insertionSort(ts []time.Time) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && ts[j].Before(ts[j-1]); j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
}
