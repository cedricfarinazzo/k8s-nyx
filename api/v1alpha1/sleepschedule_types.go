/*
Copyright 2026.

Licensed under the MIT License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Day is a day of the week an awake window applies to.
// +kubebuilder:validation:Enum=Mon;Tue;Wed;Thu;Fri;Sat;Sun
type Day string

// TargetMode selects how a SleepSchedule chooses the namespaces it acts on.
// +kubebuilder:validation:Enum=namespaces;labels
type TargetMode string

const (
	TargetModeNamespaces TargetMode = "namespaces"
	TargetModeLabels     TargetMode = "labels"
)

// AwakeWindow is a recurring window during which targets are kept awake.
type AwakeWindow struct {
	// Days this window applies to.
	// +kubebuilder:validation:MinItems=1
	Days []Day `json:"days"`

	// From is the window start, "HH:MM" 24-hour, in the schedule's timezone.
	// +kubebuilder:validation:Pattern=`^([01]\d|2[0-3]):[0-5]\d$`
	From string `json:"from"`

	// To is the window end, "HH:MM" 24-hour, in the schedule's timezone.
	// +kubebuilder:validation:Pattern=`^([01]\d|2[0-3]):[0-5]\d$`
	To string `json:"to"`
}

// Target describes which namespaces a SleepSchedule acts on.
type Target struct {
	// Mode is either "namespaces" (explicit list) or "labels" (selector).
	Mode TargetMode `json:"mode"`

	// Namespaces is the explicit namespace list when Mode is "namespaces".
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// Selector is the namespace label selector when Mode is "labels".
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// ResourceRef references a single workload by kind and name.
type ResourceRef struct {
	// Kind is the workload kind, e.g. "Deployment".
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name is the workload name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// TemporaryWake bounds the duration of ad-hoc wake overrides.
type TemporaryWake struct {
	// MaxDuration is the safety cap on a single wake (e.g. "8h").
	MaxDuration metav1.Duration `json:"maxDuration"`

	// DefaultDuration is used when a wake request omits a duration (e.g. "1h").
	DefaultDuration metav1.Duration `json:"defaultDuration"`
}

// SleepScheduleSpec defines the desired state of SleepSchedule.
type SleepScheduleSpec struct {
	// Timezone is an IANA timezone name (e.g. "Europe/Paris"). Validated by the
	// admission webhook against the IANA database.
	// +kubebuilder:validation:MinLength=1
	Timezone string `json:"timezone"`

	// Awake lists the windows during which targets are kept awake; outside all
	// windows they are asleep.
	// +kubebuilder:validation:MinItems=1
	Awake []AwakeWindow `json:"awake"`

	// Target selects the namespaces this schedule acts on.
	Target Target `json:"target"`

	// Kinds restricts the workload kinds in scope. Empty means the operator default set.
	// +optional
	Kinds []string `json:"kinds,omitempty"`

	// ExcludeRefs lists individual workloads to leave untouched.
	// +optional
	ExcludeRefs []ResourceRef `json:"excludeRefs,omitempty"`

	// SleepReplicas is the replica count applied while asleep.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	// +optional
	SleepReplicas int32 `json:"sleepReplicas,omitempty"`

	// TemporaryWake bounds ad-hoc wake overrides.
	// +optional
	TemporaryWake *TemporaryWake `json:"temporaryWake,omitempty"`

	// DryRun, when true, logs intended actions without mutating workloads.
	// +optional
	DryRun bool `json:"dryRun,omitempty"`
}

// SleepSchedulePhase is the high-level state of a SleepSchedule.
// +kubebuilder:validation:Enum=Asleep;Awake;WokenByOverride
type SleepSchedulePhase string

const (
	PhaseAsleep          SleepSchedulePhase = "Asleep"
	PhaseAwake           SleepSchedulePhase = "Awake"
	PhaseWokenByOverride SleepSchedulePhase = "WokenByOverride"
)

// SleepScheduleStatus defines the observed state of SleepSchedule.
type SleepScheduleStatus struct {
	// Phase is the current high-level state.
	// +optional
	Phase SleepSchedulePhase `json:"phase,omitempty"`

	// NextTransition is when the schedule next changes state.
	// +optional
	NextTransition *metav1.Time `json:"nextTransition,omitempty"`

	// ActiveWakes is the number of non-expired temporary wake entries.
	// +optional
	ActiveWakes int32 `json:"activeWakes,omitempty"`

	// ArgocdManaged reports whether any target is managed by ArgoCD.
	// +optional
	ArgocdManaged bool `json:"argocdManaged,omitempty"`

	// Conditions represent the latest available observations of the object's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Timezone",type=string,JSONPath=`.spec.timezone`
// +kubebuilder:printcolumn:name="Next",type=string,JSONPath=`.status.nextTransition`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SleepSchedule is the Schema for the sleepschedules API.
type SleepSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SleepScheduleSpec   `json:"spec,omitempty"`
	Status SleepScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SleepScheduleList contains a list of SleepSchedule.
type SleepScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SleepSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SleepSchedule{}, &SleepScheduleList{})
}
