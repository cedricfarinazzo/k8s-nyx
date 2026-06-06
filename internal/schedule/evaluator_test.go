/*
Copyright 2026.

Licensed under the MIT License.
*/

package schedule

import (
	"testing"
	"time"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

func mustLoad(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return loc
}

// businessHours: awake Mon–Fri 08:00–20:00 in the given timezone.
func businessHours(tz string) nyxv1alpha1.SleepScheduleSpec {
	return nyxv1alpha1.SleepScheduleSpec{
		Timezone: tz,
		Awake: []nyxv1alpha1.AwakeWindow{{
			Days: []nyxv1alpha1.Day{"Mon", "Tue", "Wed", "Thu", "Fri"},
			From: "08:00",
			To:   "20:00",
		}},
	}
}

// AC1: awake inside a window, asleep outside.
func TestEvaluate_InsideOutside(t *testing.T) {
	loc := mustLoad(t)
	spec := businessHours("Europe/Paris")

	cases := []struct {
		name      string
		at        time.Time
		wantAwake bool
	}{
		{"weekday mid-window", time.Date(2026, 6, 1, 10, 0, 0, 0, loc), true}, // Mon 10:00
		{"weekday at open", time.Date(2026, 6, 1, 8, 0, 0, 0, loc), true},     // Mon 08:00 (inclusive)
		{"weekday at close", time.Date(2026, 6, 1, 20, 0, 0, 0, loc), false},  // Mon 20:00 (exclusive)
		{"weekday before open", time.Date(2026, 6, 1, 7, 59, 0, 0, loc), false},
		{"weekday after close", time.Date(2026, 6, 1, 20, 1, 0, 0, loc), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := Evaluate(spec, c.at)
			if err != nil {
				t.Fatal(err)
			}
			if res.Awake != c.wantAwake {
				t.Fatalf("Awake = %v, want %v", res.Awake, c.wantAwake)
			}
			wantPhase := nyxv1alpha1.PhaseAsleep
			if c.wantAwake {
				wantPhase = nyxv1alpha1.PhaseAwake
			}
			if res.Phase != wantPhase {
				t.Fatalf("Phase = %v, want %v", res.Phase, wantPhase)
			}
		})
	}
}

// AC2: a day with no window (weekend) is asleep all day; next transition is Monday open.
func TestEvaluate_WeekendAsleep(t *testing.T) {
	loc := mustLoad(t)
	spec := businessHours("Europe/Paris")

	sat := time.Date(2026, 6, 6, 12, 0, 0, 0, loc) // Saturday noon
	res, err := Evaluate(spec, sat)
	if err != nil {
		t.Fatal(err)
	}
	if res.Awake {
		t.Fatalf("Saturday should be Asleep, got Awake")
	}
	wantNext := time.Date(2026, 6, 8, 8, 0, 0, 0, loc) // Monday 08:00
	if !res.NextTransition.Equal(wantNext) {
		t.Fatalf("NextTransition = %s, want %s", res.NextTransition, wantNext)
	}
}

// AC3: next transition is the next boundary in both directions.
func TestEvaluate_NextTransition(t *testing.T) {
	loc := mustLoad(t)
	spec := businessHours("Europe/Paris")

	// During the window → next flip is the window close.
	res, err := Evaluate(spec, time.Date(2026, 6, 1, 10, 0, 0, 0, loc)) // Mon 10:00
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 6, 1, 20, 0, 0, 0, loc); !res.NextTransition.Equal(want) {
		t.Fatalf("mid-window NextTransition = %s, want %s", res.NextTransition, want)
	}

	// After the window → next flip is the next day's open.
	res, err = Evaluate(spec, time.Date(2026, 6, 1, 21, 0, 0, 0, loc)) // Mon 21:00
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 6, 2, 8, 0, 0, 0, loc); !res.NextTransition.Equal(want) {
		t.Fatalf("after-window NextTransition = %s, want %s", res.NextTransition, want)
	}
}

// AC4: DST — windows anchored to wall-clock local time across both transitions.
func TestEvaluate_DST(t *testing.T) {
	loc := mustLoad(t)
	// Awake every day 01:00–05:00, straddling the DST change hour.
	spec := nyxv1alpha1.SleepScheduleSpec{
		Timezone: "Europe/Paris",
		Awake: []nyxv1alpha1.AwakeWindow{{
			Days: []nyxv1alpha1.Day{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"},
			From: "01:00",
			To:   "05:00",
		}},
	}

	// Spring forward: 2026-03-29, clocks jump 02:00 -> 03:00 (CET +01:00 -> CEST +02:00).
	springOpen := time.Date(2026, 3, 29, 1, 0, 0, 0, loc)
	if off := zoneOffset(springOpen); off != 3600 {
		t.Fatalf("01:00 before spring-forward offset = %ds, want 3600 (CET)", off)
	}
	res, err := Evaluate(spec, time.Date(2026, 3, 29, 0, 30, 0, 0, loc))
	if err != nil {
		t.Fatal(err)
	}
	if !res.NextTransition.Equal(springOpen) {
		t.Fatalf("spring NextTransition = %s, want %s", res.NextTransition, springOpen)
	}
	// At 04:00 (after the skipped hour) we are inside the window and in CEST (+02:00).
	at04 := time.Date(2026, 3, 29, 4, 0, 0, 0, loc)
	if off := zoneOffset(at04); off != 7200 {
		t.Fatalf("04:00 after spring-forward offset = %ds, want 7200 (CEST)", off)
	}
	res, err = Evaluate(spec, at04)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Awake {
		t.Fatalf("04:00 should be Awake")
	}
	if want := time.Date(2026, 3, 29, 5, 0, 0, 0, loc); !res.NextTransition.Equal(want) {
		t.Fatalf("spring close NextTransition = %s, want %s", res.NextTransition, want)
	}

	// Fall back: 2026-10-25, clocks fall 03:00 -> 02:00. The 04:00 boundary is CET (+01:00).
	fallClose := time.Date(2026, 10, 25, 5, 0, 0, 0, loc)
	if off := zoneOffset(fallClose); off != 3600 {
		t.Fatalf("05:00 after fall-back offset = %ds, want 3600 (CET)", off)
	}
	res, err = Evaluate(spec, time.Date(2026, 10, 25, 4, 30, 0, 0, loc))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Awake {
		t.Fatalf("04:30 during fall-back should be Awake (inside 01:00-05:00)")
	}
	if !res.NextTransition.Equal(fallClose) {
		t.Fatalf("fall close NextTransition = %s, want %s", res.NextTransition, fallClose)
	}
}

func TestEvaluate_InvalidTimezone(t *testing.T) {
	spec := businessHours("Bogus/Zone")
	if _, err := Evaluate(spec, time.Now()); err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}

func zoneOffset(t time.Time) int {
	_, off := t.Zone()
	return off
}
