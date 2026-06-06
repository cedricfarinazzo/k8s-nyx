/*
Copyright 2026.

Licensed under the MIT License.
*/

package wake

import (
	"testing"
	"time"
)

// AC1/AC2: a valid absolute RFC3339 expiry is parsed; by/reason are optional.
func TestParseEntry_Absolute(t *testing.T) {
	e, err := ParseEntry("alice-1", "2026-06-05T15:00:00+02:00;by=alice;reason=debugging")
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
	if e.By != "alice" || e.Reason != "debugging" {
		t.Fatalf("by/reason = %q/%q, want alice/debugging", e.By, e.Reason)
	}
	if e.Relative != nil {
		t.Fatalf("relative should be nil for an absolute value")
	}
}

func TestParseEntry_AbsoluteNoAttribution(t *testing.T) {
	e, err := ParseEntry("k", "2026-06-05T15:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if e.Expiry == nil || e.By != "" || e.Reason != "" {
		t.Fatalf("by/reason should be empty and expiry set, got %+v", e)
	}
}

// AC1: a relative +duration is recognized (kept unresolved).
func TestParseEntry_Relative(t *testing.T) {
	e, err := ParseEntry("bob", "+1h;by=bob;reason=quick test")
	if err != nil {
		t.Fatal(err)
	}
	if e.Relative == nil || *e.Relative != time.Hour {
		t.Fatalf("relative = %v, want 1h", e.Relative)
	}
	if e.Expiry != nil {
		t.Fatalf("expiry should be nil for a relative value")
	}
	if e.By != "bob" || e.Reason != "quick test" {
		t.Fatalf("by/reason = %q/%q", e.By, e.Reason)
	}
}

// AC3: malformed values are errors (the caller drops them).
func TestParseEntry_Malformed(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"bad timestamp", "not-a-time;by=x"},
		{"garbage", "????"},
		{"bad duration", "+nonsense"},
		{"zero duration", "+0s"},
		{"negative duration", "-1h"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseEntry("k", c.value); err == nil {
				t.Fatalf("expected error for %q", c.value)
			}
		})
	}
}

// A head-less value (empty or attribute-only) is a valid "no expiry" entry —
// the caller applies the default duration.
func TestParseEntry_NoExpiry(t *testing.T) {
	for _, v := range []string{"", "by=alice;reason=lunch", ";by=bob"} {
		e, err := ParseEntry("k", v)
		if err != nil {
			t.Fatalf("ParseEntry(%q) error: %v", v, err)
		}
		if e.HasExpiry() {
			t.Fatalf("value %q should have no expiry", v)
		}
	}
	e, _ := ParseEntry("k", "by=alice;reason=lunch")
	if e.By != "alice" || e.Reason != "lunch" {
		t.Fatalf("attribution not parsed from head-less value: %+v", e)
	}
}

// ParseData splits valid from malformed and names the offending keys.
func TestParseData(t *testing.T) {
	data := map[string]string{
		"good-1": "2026-06-05T15:00:00Z;by=alice",
		"good-2": "+30m;reason=deploy",
		"bad-1":  "not-a-time",
	}
	valid, errs := ParseData(data)
	if len(valid) != 2 {
		t.Fatalf("valid = %d, want 2", len(valid))
	}
	if _, ok := errs["bad-1"]; !ok {
		t.Fatalf("expected bad-1 in errors, got %v", errs)
	}
	if len(errs) != 1 {
		t.Fatalf("errs = %d, want 1", len(errs))
	}
	// sorted by key
	if valid[0].Key != "good-1" || valid[1].Key != "good-2" {
		t.Fatalf("entries not sorted by key: %q, %q", valid[0].Key, valid[1].Key)
	}
}
