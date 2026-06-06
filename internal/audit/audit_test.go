/*
Copyright 2026.

Licensed under the MIT License.
*/

package audit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/cedricfarinazzo/k8s-nyx/internal/audit"
)

// jsonLogCtx returns a context whose logger writes JSON to buf.
func jsonLogCtx(buf *bytes.Buffer) context.Context {
	logger := crzap.New(crzap.WriteTo(buf), crzap.UseDevMode(false))
	return logf.IntoContext(context.Background(), logger)
}

func lastJSONLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &m); err != nil {
		t.Fatalf("log line is not JSON (%v): %q", err, lines[len(lines)-1])
	}
	return m
}

func schedule() *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "team-a", Name: "dev-hours"}}
}

// AC1 + AC3: the audit log is JSON and carries who / what (action) / why / when
// plus the affected object ref for correlation.
func TestRecord_StructuredJSONLog(t *testing.T) {
	var buf bytes.Buffer
	ctx := audit.NewContext(jsonLogCtx(&buf), audit.Info{Who: "alice", Why: "active wake override"})

	audit.Record(ctx, nil, schedule(), "Deployment", "team-a", "api", "WakeOverride", "active")

	m := lastJSONLine(t, &buf)
	if m["action"] != "WakeOverride" {
		t.Fatalf("action = %v, want WakeOverride", m["action"])
	}
	if m["who"] != "alice" {
		t.Fatalf("who = %v, want alice", m["who"])
	}
	if m["why"] != "active wake override" {
		t.Fatalf("why = %v, want active wake override", m["why"])
	}
	if m["objectRef"] != "Deployment/team-a/api" {
		t.Fatalf("objectRef = %v, want Deployment/team-a/api", m["objectRef"])
	}
	if _, ok := m["when"].(string); !ok || m["when"] == "" {
		t.Fatalf("when missing/empty: %v", m["when"])
	}
	if m["sleepSchedule"] != "team-a/dev-hours" {
		t.Fatalf("sleepSchedule = %v, want team-a/dev-hours", m["sleepSchedule"])
	}
}

// AC2: Record posts an Event on the SleepSchedule, naming the action + ref.
func TestRecord_EmitsScheduleEvent(t *testing.T) {
	var buf bytes.Buffer
	ctx := audit.NewContext(jsonLogCtx(&buf), audit.Info{Why: "asleep window"})
	rec := record.NewFakeRecorder(5)

	audit.Record(ctx, rec, schedule(), "StatefulSet", "team-a", "db", "Slept", "scaled to 0 replicas")

	select {
	case e := <-rec.Events:
		if !strings.Contains(e, "Slept") || !strings.Contains(e, "StatefulSet/team-a/db") {
			t.Fatalf("event = %q, want Slept + objectRef", e)
		}
	default:
		t.Fatal("no Event recorded on the SleepSchedule")
	}
}

// who defaults to the operator when unset.
func TestFromContext_DefaultActor(t *testing.T) {
	if got := audit.FromContext(context.Background()).Who; got != audit.DefaultActor {
		t.Fatalf("default who = %q, want %q", got, audit.DefaultActor)
	}
	got := audit.FromContext(audit.NewContext(context.Background(), audit.Info{Who: "bob"})).Who
	if got != "bob" {
		t.Fatalf("who = %q, want bob", got)
	}
}
