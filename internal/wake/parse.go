/*
Copyright 2026.

Licensed under the MIT License.
*/

// Package wake parses and resolves the single wake-override value the operator
// reads from a schedule's wake ConfigMap. The override is just an expiry — an
// RFC3339 timestamp, a relative "+duration", or empty (use the schedule's
// temporaryWake.defaultDuration). There is no key, author, or reason: the
// ConfigMap holds one value under a well-known key and that value is the expiry.
package wake

import (
	"fmt"
	"strings"
	"time"
)

// Entry is a parsed wake override. At most one of Expiry / Relative is set; both
// nil means "no expiry given" (resolve applies the default duration).
type Entry struct {
	// Expiry is the absolute expiry when the value is an RFC3339 timestamp.
	Expiry *time.Time
	// Relative is the unresolved duration when the value is "+<duration>".
	Relative *time.Duration
}

// ParseEntry parses a wake-override value. The value is one of:
//
//   - "+<duration>"      — relative, e.g. "+2h", "+90m", "+1h30m"
//   - an RFC3339 stamp   — absolute, e.g. "2026-06-05T20:00:00Z"
//   - "" (empty)         — no expiry; resolve applies temporaryWake.defaultDuration
//
// Anything else is an error; the caller surfaces it as a Warning and ignores it.
func ParseEntry(value string) (Entry, error) {
	head := strings.TrimSpace(value)
	var e Entry
	switch {
	case head == "":
		// No expiry: the default duration is applied at resolve time.
	case strings.HasPrefix(head, "+"):
		d, err := time.ParseDuration(head)
		if err != nil {
			return Entry{}, fmt.Errorf("invalid relative duration %q: %w", head, err)
		}
		if d <= 0 {
			return Entry{}, fmt.Errorf("relative duration must be positive: %q", head)
		}
		e.Relative = &d
	default:
		t, err := time.Parse(time.RFC3339, head)
		if err != nil {
			return Entry{}, fmt.Errorf("invalid expiry %q: not RFC3339 or +duration", head)
		}
		e.Expiry = &t
	}
	return e, nil
}

// HasExpiry reports whether an explicit expiry (absolute or relative) was given.
func (e Entry) HasExpiry() bool {
	return e.Expiry != nil || e.Relative != nil
}
