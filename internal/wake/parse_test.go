/*
Copyright 2026.

Licensed under the MIT License.
*/

package wake

import (
	"testing"
	"time"
)

func TestParseEntry_Absolute(t *testing.T) {
	e, err := ParseEntry("2026-06-05T15:00:00+02:00")
	if err != nil {
		t.Fatal(err)
	}
	if e.Expiry == nil {
		t.Fatal("expected an absolute expiry")
	}
	want := time.Date(2026, 6, 5, 15, 0, 0, 0, time.FixedZone("", 2*3600))
	if !e.Expiry.Equal(want) {
		t.Fatalf("expiry = %s, want %s", e.Expiry, want)
	}
	if e.Relative != nil {
		t.Fatalf("relative should be nil for an absolute value")
	}
}

func TestParseEntry_Relative(t *testing.T) {
	e, err := ParseEntry("+1h30m")
	if err != nil {
		t.Fatal(err)
	}
	if e.Relative == nil || *e.Relative != 90*time.Minute {
		t.Fatalf("relative = %v, want 1h30m", e.Relative)
	}
	if e.Expiry != nil {
		t.Fatalf("expiry should be nil for a relative value")
	}
}

func TestParseEntry_Empty(t *testing.T) {
	for _, v := range []string{"", "   "} {
		e, err := ParseEntry(v)
		if err != nil {
			t.Fatalf("ParseEntry(%q) error: %v", v, err)
		}
		if e.HasExpiry() {
			t.Fatalf("value %q should have no expiry", v)
		}
	}
}

func TestParseEntry_Malformed(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"bad timestamp", "not-a-time"},
		{"garbage", "????"},
		{"bad duration", "+nonsense"},
		{"zero duration", "+0s"},
		{"negative duration", "-1h"},
		{"stray attributes", "+2h;by=alice"}, // attributes are no longer supported
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseEntry(c.value); err == nil {
				t.Fatalf("expected error for %q", c.value)
			}
		})
	}
}
