/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ExperimentSpec defines the desired state of Experiment
type ExperimentSpec struct {
	// ExperimentTemplate specifies which template to use
	// Either ID or Name must be specified
	// +required
	ExperimentTemplate ExperimentTemplateRef `json:"experimentTemplate"`

	// Schedule defines when to run the experiment (cron expression)
	// If not specified, the experiment runs once immediately (Job mode)
	// Examples: "0 2 * * *" (daily at 2am), "*/30 * * * *" (every 30 minutes)
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Suspend tells the controller to suspend subsequent executions
	// This does not apply to already started experiments
	// +optional
	Suspend *bool `json:"suspend,omitempty"`

	// SuccessfulExperimentsHistoryLimit is the number of successful finished experiments to retain
	// Default is 3
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=3
	// +optional
	SuccessfulExperimentsHistoryLimit *int32 `json:"successfulExperimentsHistoryLimit,omitempty"`

	// FailedExperimentsHistoryLimit is the number of failed finished experiments to retain
	// Default is 1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	FailedExperimentsHistoryLimit *int32 `json:"failedExperimentsHistoryLimit,omitempty"`

	// Tags to apply to the experiment
	// +optional
	Tags []Tag `json:"tags,omitempty"`

	// ClientToken is an optional unique identifier for the experiment
	// If not provided, one will be generated automatically
	// +optional
	ClientToken string `json:"clientToken,omitempty"`
}

// ExperimentTemplateRef references an experiment template by ID or Name
type ExperimentTemplateRef struct {
	// ID is the AWS FIS experiment template ID (e.g., "EXT1234567890abcdef")
	// Either ID or Name must be specified
	// +optional
	ID string `json:"id,omitempty"`

	// Name is the name of the ExperimentTemplate CRD in the same namespace
	// Either ID or Name must be specified
	// +optional
	Name string `json:"name,omitempty"`
}

// ExperimentStatus defines the observed state of Experiment.
type ExperimentStatus struct {
	// ExperimentID is the AWS FIS experiment ID
	// +optional
	ExperimentID string `json:"experimentId,omitempty"`

	// TemplateID is the resolved AWS FIS template ID
	// +optional
	TemplateID string `json:"templateId,omitempty"`

	// State represents the current state of the experiment
	// Possible values: initiating, pending, running, completed, stopping, stopped, failed
	// +optional
	State string `json:"state,omitempty"`

	// Reason provides additional information about the current state
	// +optional
	Reason string `json:"reason,omitempty"`

	// StartTime is when the experiment started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// EndTime is when the experiment ended
	// +optional
	EndTime *metav1.Time `json:"endTime,omitempty"`

	// LastScheduleTime is the last time the experiment was scheduled (for scheduled experiments)
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// NextScheduleTime is the next time the experiment will be scheduled (for scheduled experiments)
	// +optional
	NextScheduleTime *metav1.Time `json:"nextScheduleTime,omitempty"`

	// Active is the number of currently running experiments
	// +optional
	Active int32 `json:"active,omitempty"`

	// TargetAccountConfigurationsCount is the number of target account configurations
	// +optional
	TargetAccountConfigurationsCount int64 `json:"targetAccountConfigurationsCount,omitempty"`

	// Conditions represent the current state of the Experiment resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=fisexp
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Experiment ID",type=string,JSONPath=`.status.experimentId`
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.experimentTemplate.name`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Last Schedule",type=date,JSONPath=`.status.lastScheduleTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Experiment is the Schema for the experiments API
type Experiment struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of Experiment
	// +required
	Spec ExperimentSpec `json:"spec"`

	// status defines the observed state of Experiment
	// +optional
	Status ExperimentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ExperimentList contains a list of Experiment
type ExperimentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Experiment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Experiment{}, &ExperimentList{})
}
