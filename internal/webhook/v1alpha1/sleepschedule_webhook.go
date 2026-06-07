/*
Copyright 2026.

Licensed under the MIT License.
*/

package v1alpha1

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

var sleepschedulelog = logf.Log.WithName("sleepschedule-webhook")

// SetupSleepScheduleWebhookWithManager registers the validating webhook.
func SetupSleepScheduleWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &nyxv1alpha1.SleepSchedule{}).
		WithValidator(&SleepScheduleCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-nyx-dev-v1alpha1-sleepschedule,mutating=false,failurePolicy=fail,sideEffects=None,groups=nyx.dev,resources=sleepschedules,verbs=create;update,versions=v1alpha1,name=vsleepschedule-v1alpha1.kb.io,admissionReviewVersions=v1

// SleepScheduleCustomValidator validates SleepSchedule resources beyond what the
// OpenAPI schema can express (IANA timezone, window ordering, target consistency).
type SleepScheduleCustomValidator struct{}

// As of controller-runtime v0.23 CustomValidator is generic over the validated
// type, so the methods receive a typed *SleepSchedule directly (no runtime.Object
// type assertion needed).
var _ admission.Validator[*nyxv1alpha1.SleepSchedule] = &SleepScheduleCustomValidator{}

func (v *SleepScheduleCustomValidator) ValidateCreate(_ context.Context, ss *nyxv1alpha1.SleepSchedule) (admission.Warnings, error) {
	sleepschedulelog.V(1).Info("validate create", "name", ss.GetName())
	return nil, validateSleepSchedule(ss)
}

func (v *SleepScheduleCustomValidator) ValidateUpdate(_ context.Context, _, newObj *nyxv1alpha1.SleepSchedule) (admission.Warnings, error) {
	sleepschedulelog.V(1).Info("validate update", "name", newObj.GetName())
	return nil, validateSleepSchedule(newObj)
}

func (v *SleepScheduleCustomValidator) ValidateDelete(_ context.Context, _ *nyxv1alpha1.SleepSchedule) (admission.Warnings, error) {
	return nil, nil
}

// validateSleepSchedule runs every cross-field / data check the OpenAPI schema
// cannot, and aggregates them into a single apierrors.Invalid status.
func validateSleepSchedule(ss *nyxv1alpha1.SleepSchedule) error {
	var allErrs field.ErrorList
	spec := ss.Spec
	specPath := field.NewPath("spec")

	// IANA timezone — the reason this webhook exists; OpenAPI cannot check it.
	if _, err := time.LoadLocation(spec.Timezone); err != nil {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("timezone"), spec.Timezone,
			"must be a valid IANA timezone (e.g. \"Europe/Paris\")"))
	}

	// Awake windows: "from" must be strictly before "to". The OpenAPI pattern
	// already guarantees zero-padded "HH:MM", so a lexical compare is correct.
	for i, w := range spec.Awake {
		if w.From >= w.To {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("awake").Index(i).Child("from"), w.From,
				fmt.Sprintf("must be earlier than \"to\" (%s)", w.To)))
		}
	}

	// Target mode ↔ field consistency.
	switch spec.Target.Mode {
	case nyxv1alpha1.TargetModeNamespaces:
		if len(spec.Target.Namespaces) == 0 {
			allErrs = append(allErrs, field.Required(
				specPath.Child("target").Child("namespaces"),
				"at least one namespace is required when target.mode is \"namespaces\""))
		}
	case nyxv1alpha1.TargetModeLabels:
		if spec.Target.Selector == nil {
			allErrs = append(allErrs, field.Required(
				specPath.Child("target").Child("selector"),
				"a selector is required when target.mode is \"labels\""))
		}
	}

	// Temporary-wake durations must be positive and default ≤ max.
	if tw := spec.TemporaryWake; tw != nil {
		twPath := specPath.Child("temporaryWake")
		if tw.MaxDuration.Duration <= 0 {
			allErrs = append(allErrs, field.Invalid(
				twPath.Child("maxDuration"), tw.MaxDuration.Duration.String(), "must be a positive duration"))
		}
		if tw.DefaultDuration.Duration <= 0 {
			allErrs = append(allErrs, field.Invalid(
				twPath.Child("defaultDuration"), tw.DefaultDuration.Duration.String(), "must be a positive duration"))
		}
		if tw.MaxDuration.Duration > 0 && tw.DefaultDuration.Duration > tw.MaxDuration.Duration {
			allErrs = append(allErrs, field.Invalid(
				twPath.Child("defaultDuration"), tw.DefaultDuration.Duration.String(),
				"must not exceed temporaryWake.maxDuration"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: nyxv1alpha1.GroupVersion.Group, Kind: "SleepSchedule"},
		ss.Name, allErrs)
}
