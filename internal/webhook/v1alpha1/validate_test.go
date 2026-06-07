/*
Copyright 2026.

Licensed under the MIT License.
*/

package v1alpha1

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

func validSchedule() *nyxv1alpha1.SleepSchedule {
	return &nyxv1alpha1.SleepSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
		Spec: nyxv1alpha1.SleepScheduleSpec{
			Timezone: "Europe/Paris",
			Awake: []nyxv1alpha1.AwakeWindow{
				{Days: []nyxv1alpha1.Day{"Mon"}, From: "08:00", To: "20:00"},
			},
			Target: nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
		},
	}
}

func TestValidator_CreateUpdateDelete(t *testing.T) {
	v := &SleepScheduleCustomValidator{}
	ctx := context.Background()

	if _, err := v.ValidateCreate(ctx, validSchedule()); err != nil {
		t.Fatalf("valid create: %v", err)
	}
	if _, err := v.ValidateUpdate(ctx, validSchedule(), validSchedule()); err != nil {
		t.Fatalf("valid update: %v", err)
	}
	if _, err := v.ValidateDelete(ctx, validSchedule()); err != nil {
		t.Fatalf("delete should never error: %v", err)
	}
	// The validator is now generic over *SleepSchedule, so a wrong object type is a
	// compile-time error rather than a runtime check — no negative case needed.
}

func TestValidator_Rules(t *testing.T) {
	v := &SleepScheduleCustomValidator{}
	ctx := context.Background()

	cases := []struct {
		name   string
		mutate func(*nyxv1alpha1.SleepSchedule)
		want   string
	}{
		{"bad timezone", func(s *nyxv1alpha1.SleepSchedule) { s.Spec.Timezone = "Bogus/Zone" }, "IANA timezone"},
		{"from after to", func(s *nyxv1alpha1.SleepSchedule) { s.Spec.Awake[0].From = "20:00"; s.Spec.Awake[0].To = "08:00" }, "earlier than"},
		{"namespaces mode without namespaces", func(s *nyxv1alpha1.SleepSchedule) {
			s.Spec.Target = nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces}
		}, "namespace is required"},
		{"labels mode without selector", func(s *nyxv1alpha1.SleepSchedule) {
			s.Spec.Target = nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeLabels}
		}, "selector is required"},
		{"non-positive max duration", func(s *nyxv1alpha1.SleepSchedule) {
			s.Spec.TemporaryWake = &nyxv1alpha1.TemporaryWake{MaxDuration: metav1.Duration{}, DefaultDuration: metav1.Duration{}}
		}, "positive duration"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := validSchedule()
			c.mutate(s)
			_, err := v.ValidateCreate(ctx, s)
			if err == nil {
				t.Fatalf("expected error containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}

	// default > max is rejected.
	s := validSchedule()
	s.Spec.TemporaryWake = &nyxv1alpha1.TemporaryWake{
		MaxDuration:     metav1.Duration{Duration: 1e9}, // 1s
		DefaultDuration: metav1.Duration{Duration: 2e9}, // 2s
	}
	if _, err := v.ValidateCreate(ctx, s); err == nil || !strings.Contains(err.Error(), "exceed") {
		t.Fatalf("expected default>max rejection, got %v", err)
	}
}
