/*
Copyright 2026.

Licensed under the MIT License.
*/

package wake

import (
	"fmt"
	"strings"
	"time"
)

// Resolution is the outcome of resolving an entry to an absolute expiry.
type Resolution struct {
	// Expiry is the absolute expiry the entry resolves to.
	Expiry time.Time
	// Changed is true when the stored value must be rewritten (a relative or
	// no-expiry entry was stamped to absolute, or an over-cap expiry was clamped).
	Changed bool
	// Clamped is true when the expiry was reduced to now+max.
	Clamped bool
}

// Resolve turns an entry into an absolute expiry, applying the default duration
// for a no-expiry entry and clamping anything beyond now+max.
//
//   - relative "+d"      -> now + d                 (Changed: stamp to absolute)
//   - no expiry given    -> now + def               (Changed: stamp to absolute)
//   - absolute timestamp -> as written              (Changed only if clamped)
//
// def/max of 0 mean "not configured" (temporaryWake absent): no default is
// available, and no cap is applied.
func Resolve(e Entry, now time.Time, def, max time.Duration) (Resolution, error) {
	var exp time.Time
	var changed bool

	switch {
	case e.Relative != nil:
		exp = now.Add(*e.Relative)
		changed = true // stamp the relative value to absolute so it never re-extends
	case e.Expiry != nil:
		exp = *e.Expiry
	default:
		if def <= 0 {
			return Resolution{}, fmt.Errorf("no expiry and no temporaryWake.defaultDuration configured")
		}
		exp = now.Add(def)
		changed = true
	}

	res := Resolution{Expiry: exp, Changed: changed}
	if max > 0 {
		if capTime := now.Add(max); exp.After(capTime) {
			res.Expiry = capTime
			res.Changed = true
			res.Clamped = true
		}
	}
	return res, nil
}

// FormatEntry renders an absolute wake entry value: "<RFC3339>[;by=…][;reason=…]".
// It is the canonical written-back form once an entry has been resolved.
func FormatEntry(expiry time.Time, by, reason string) string {
	var b strings.Builder
	b.WriteString(expiry.UTC().Format(time.RFC3339))
	if by != "" {
		b.WriteString(";by=")
		b.WriteString(by)
	}
	if reason != "" {
		b.WriteString(";reason=")
		b.WriteString(reason)
	}
	return b.String()
}
