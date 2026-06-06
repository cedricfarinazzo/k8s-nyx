/*
Copyright 2026.

Licensed under the MIT License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SleepScheduleSpec defines the desired state of SleepSchedule.
//
// This is a placeholder scaffold (VC-120). Real reconcile fields are added by E2.
type SleepScheduleSpec struct {
	// Suspend pauses the schedule when true. Placeholder field.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// SleepScheduleStatus defines the observed state of SleepSchedule.
type SleepScheduleStatus struct {
	// Conditions represent the latest available observations of the object's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced

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
