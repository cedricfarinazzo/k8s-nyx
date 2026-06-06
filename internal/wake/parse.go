/*
Copyright 2026.

Licensed under the MIT License.
*/

// Package wake parses the entries written into a schedule's Wake ConfigMap.
// A data value is "<expiry-or-+duration>[;by=<who>;reason=<text>]" — for example
// "2026-06-05T15:00:00+02:00;by=alice;reason=debugging" or "+1h;by=bob".
// Resolving relative durations and honouring entries are separate stories; this
// package only parses, validates, and surfaces malformed input.
package wake

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Entry is a parsed wake override. Exactly one of Expiry / Relative is set.
type Entry struct {
	// Key is the ConfigMap data key the entry came from.
	Key string
	// Expiry is the absolute expiry when the value is an RFC3339 timestamp.
	Expiry *time.Time
	// Relative is the unresolved relative duration when the value is "+<duration>".
	// Resolving it to an absolute time (and clamping) is a separate story.
	Relative *time.Duration
	// By and Reason are optional attribution for the audit trail.
	By     string
	Reason string
}

// ParseEntry parses a single Wake ConfigMap data value. A malformed head segment
// (neither a valid RFC3339 timestamp nor a positive "+<duration>") is an error;
// the caller drops the entry and surfaces the offending key.
func ParseEntry(key, value string) (Entry, error) {
	segments := strings.Split(value, ";")
	head := strings.TrimSpace(segments[0])
	if head == "" {
		return Entry{}, fmt.Errorf("empty wake value")
	}

	e := Entry{Key: key}
	switch {
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

	for _, seg := range segments[1:] {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		k, v, ok := strings.Cut(seg, "=")
		if !ok {
			continue // ignore an attribute that is not key=value (lenient)
		}
		switch strings.TrimSpace(k) {
		case "by":
			e.By = strings.TrimSpace(v)
		case "reason":
			e.Reason = strings.TrimSpace(v)
		}
	}
	return e, nil
}

// ParseData parses every entry in a ConfigMap's data. It returns the valid entries
// (sorted by key for deterministic processing) and a map of key → parse error for
// the malformed ones, so the caller can drop them and emit Warning Events.
func ParseData(data map[string]string) (valid []Entry, errs map[string]error) {
	errs = map[string]error{}
	for k, v := range data {
		e, err := ParseEntry(k, v)
		if err != nil {
			errs[k] = err
			continue
		}
		valid = append(valid, e)
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].Key < valid[j].Key })
	return valid, errs
}
