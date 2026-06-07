/*
Copyright 2026.

Licensed under the MIT License.
*/

package wake

import (
	"testing"
	"time"
)

func dur(d time.Duration) *time.Duration { return &d }
func tm(t time.Time) *time.Time          { return &t }

var base = time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)

// AC1: a relative duration resolves to now+d and is marked Changed (stamped).
func TestResolve_Relative(t *testing.T) {
	res, err := Resolve(Entry{Relative: dur(time.Hour)}, base, time.Hour, 8*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Expiry.Equal(base.Add(time.Hour)) {
		t.Fatalf("expiry = %s, want %s", res.Expiry, base.Add(time.Hour))
	}
	if !res.Changed || res.Clamped {
		t.Fatalf("expected changed && !clamped, got %+v", res)
	}
}

// AC2: a no-expiry entry gets now + defaultDuration.
func TestResolve_Default(t *testing.T) {
	res, err := Resolve(Entry{}, base, 2*time.Hour, 8*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Expiry.Equal(base.Add(2 * time.Hour)) {
		t.Fatalf("expiry = %s, want %s", res.Expiry, base.Add(2*time.Hour))
	}
	if !res.Changed {
		t.Fatalf("expected changed")
	}
}

func TestResolve_NoDefaultErrors(t *testing.T) {
	if _, err := Resolve(Entry{}, base, 0, 0); err == nil {
		t.Fatal("expected error when no expiry and no default")
	}
}

// An absolute expiry within the cap is left unchanged (AC1 — no re-extend).
func TestResolve_AbsoluteWithinCap(t *testing.T) {
	exp := base.Add(3 * time.Hour)
	res, err := Resolve(Entry{Expiry: tm(exp)}, base, time.Hour, 8*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || res.Clamped {
		t.Fatalf("absolute within cap should be unchanged, got %+v", res)
	}
	if !res.Expiry.Equal(exp) {
		t.Fatalf("expiry = %s, want %s", res.Expiry, exp)
	}
}

// AC3: an expiry beyond now+max is clamped to the cap and marked Changed+Clamped.
func TestResolve_ClampOverCap(t *testing.T) {
	exp := base.Add(100 * time.Hour)
	res, err := Resolve(Entry{Expiry: tm(exp)}, base, time.Hour, 8*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Clamped || !res.Changed {
		t.Fatalf("expected clamped+changed, got %+v", res)
	}
	if !res.Expiry.Equal(base.Add(8 * time.Hour)) {
		t.Fatalf("clamped expiry = %s, want %s", res.Expiry, base.Add(8*time.Hour))
	}
}

func TestFormatExpiry(t *testing.T) {
	exp := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if got := FormatExpiry(exp); got != "2026-06-01T12:00:00Z" {
		t.Fatalf("FormatExpiry = %q", got)
	}

	// Round-trip: a formatted value parses back to the same absolute expiry.
	e, err := ParseEntry(FormatExpiry(exp))
	if err != nil || e.Expiry == nil || !e.Expiry.Equal(exp) {
		t.Fatalf("round-trip failed: %+v err=%v", e, err)
	}
}
