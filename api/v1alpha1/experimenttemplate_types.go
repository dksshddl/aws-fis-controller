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

// ExperimentTemplateSpec defines the desired state of ExperimentTemplate
type ExperimentTemplateSpec struct {
	// Description of the experiment template
	// +optional
	Description string `json:"description,omitempty"`

	// Targets defines which pods to target for the experiment
	// +kubebuilder:validation:MinItems=1
	// +required
	Targets []TargetSpec `json:"targets"`

	// Actions defines the chaos actions to perform
	// +kubebuilder:validation:MinItems=1
	// +required
	Actions []ActionSpec `json:"actions"`

	// StopConditions defines conditions that will stop the experiment
	// +optional
	StopConditions []StopCondition `json:"stopConditions,omitempty"`

	// ExperimentOptions defines experiment-level options
	// +optional
	ExperimentOptions *ExperimentOptions `json:"experimentOptions,omitempty"`

	// LogConfiguration defines where to send experiment logs
	// +optional
	LogConfiguration *LogConfiguration `json:"logConfiguration,omitempty"`

	// ExperimentReportConfiguration defines experiment report settings
	// +optional
	ExperimentReportConfiguration *ExperimentReportConfiguration `json:"experimentReportConfiguration,omitempty"`

	// Tags to apply to the FIS experiment template
	// +optional
	Tags []Tag `json:"tags,omitempty"`
}

// TargetSpec defines the target pods for the experiment
type TargetSpec struct {
	// Name is a unique identifier for this target
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9-]+$`
	// +required
	Name string `json:"name"`

	// Namespace where the target pods are located
	// +kubebuilder:default=default
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// LabelSelector to select target pods (key-value pairs)
	// +required
	LabelSelector map[string]string `json:"labelSelector"`

	// SelectionMode defines how to select pods (ALL, COUNT, PERCENT)
	// +kubebuilder:validation:Enum=ALL;COUNT;PERCENT
	// +kubebuilder:default=ALL
	// +optional
	SelectionMode string `json:"selectionMode,omitempty"`

	// Count specifies the number of pods to select when SelectionMode is COUNT
	// +kubebuilder:validation:Minimum=1
	// +optional
	Count *int `json:"count,omitempty"`

	// Percent specifies the percentage of pods to select when SelectionMode is PERCENT
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	Percent *int `json:"percent,omitempty"`

	// TargetContainerName specifies which container in the pod to target
	// +optional
	TargetContainerName string `json:"targetContainerName,omitempty"`

	// Filters for additional target selection criteria
	// TODO: Implement filter support for advanced target selection
	// +optional
	Filters []TargetFilter `json:"filters,omitempty"`
}

// TargetFilter defines additional filtering criteria for target selection
type TargetFilter struct {
	// Path is the JSON path to filter on
	// +required
	Path string `json:"path"`

	// Values are the values to match
	// +required
	Values []string `json:"values"`
}

// ActionSpec defines a chaos action to perform
type ActionSpec struct {
	// Name is a unique identifier for this action
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9-]+$`
	// +required
	Name string `json:"name"`

	// Description of the action
	// +optional
	Description string `json:"description,omitempty"`

	// Type is the action type (pod-cpu-stress, pod-memory-stress, pod-io-stress, pod-network-latency, etc.)
	// +kubebuilder:validation:Enum=pod-cpu-stress;pod-memory-stress;pod-io-stress;pod-network-latency;pod-network-packet-loss;pod-delete
	// +required
	Type string `json:"type"`

	// Duration of the action (e.g., "5m", "10m", "1h")
	// +kubebuilder:validation:Pattern=`^\d+[smh]$`
	// +required
	Duration string `json:"duration"`

	// Parameters for the action (e.g., percent, delayMilliseconds)
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`

	// Target is the name of the target to apply this action to
	// +required
	Target string `json:"target"`

	// StartAfter lists action names that must complete before this action starts
	// +optional
	StartAfter []string `json:"startAfter,omitempty"`
}

// StopCondition defines a condition that will stop the experiment
type StopCondition struct {
	// Source is the source of the stop condition (e.g., "cloudwatch-alarm", "none")
	// +kubebuilder:validation:Enum=cloudwatch-alarm;none
	// +required
	Source string `json:"source"`

	// Value is the ARN of the CloudWatch alarm (required when source is cloudwatch-alarm)
	// +optional
	Value string `json:"value,omitempty"`
}

// ExperimentOptions defines experiment-level options
type ExperimentOptions struct {
	// AccountTargeting defines the account targeting mode
	// +kubebuilder:validation:Enum=single-account;multi-account
	// +kubebuilder:default=single-account
	// +optional
	AccountTargeting string `json:"accountTargeting,omitempty"`

	// EmptyTargetResolutionMode defines behavior when no targets are found
	// +kubebuilder:validation:Enum=fail;skip
	// +kubebuilder:default=fail
	// +optional
	EmptyTargetResolutionMode string `json:"emptyTargetResolutionMode,omitempty"`
}

// LogConfiguration defines where to send experiment logs
type LogConfiguration struct {
	// LogSchemaVersion is the schema version for logs
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=2
	// +optional
	LogSchemaVersion int `json:"logSchemaVersion,omitempty"`

	// CloudWatchLogsConfiguration defines CloudWatch Logs settings
	// +optional
	CloudWatchLogsConfiguration *CloudWatchLogsConfiguration `json:"cloudWatchLogsConfiguration,omitempty"`

	// S3Configuration defines S3 logging settings
	// +optional
	S3Configuration *S3Configuration `json:"s3Configuration,omitempty"`
}

// CloudWatchLogsConfiguration defines CloudWatch Logs settings
type CloudWatchLogsConfiguration struct {
	// LogGroupArn is the ARN of the CloudWatch log group
	// +kubebuilder:validation:Pattern=`^arn:aws:logs:[a-z0-9-]+:\d{12}:log-group:.+$`
	// +required
	LogGroupArn string `json:"logGroupArn"`
}

// S3Configuration defines S3 settings for logs or reports
type S3Configuration struct {
	// BucketName is the name of the S3 bucket
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=63
	// +required
	BucketName string `json:"bucketName"`

	// Prefix is the S3 key prefix
	// +optional
	Prefix string `json:"prefix,omitempty"`
}

// ExperimentReportConfiguration defines experiment report settings
type ExperimentReportConfiguration struct {
	// PreExperimentDuration is the duration before the experiment to include in the report (e.g., "20m")
	// +kubebuilder:validation:Pattern=`^\d+[smh]$`
	// +optional
	PreExperimentDuration string `json:"preExperimentDuration,omitempty"`

	// PostExperimentDuration is the duration after the experiment to include in the report (e.g., "20m")
	// +kubebuilder:validation:Pattern=`^\d+[smh]$`
	// +optional
	PostExperimentDuration string `json:"postExperimentDuration,omitempty"`

	// DataSources defines data sources for the report
	// +optional
	DataSources *ReportDataSources `json:"dataSources,omitempty"`

	// Outputs defines where to store the report
	// +optional
	Outputs *ReportOutputs `json:"outputs,omitempty"`
}

// ReportDataSources defines data sources for experiment reports
type ReportDataSources struct {
	// CloudWatchDashboards is a list of CloudWatch dashboard ARNs
	// +optional
	CloudWatchDashboards []CloudWatchDashboard `json:"cloudWatchDashboards,omitempty"`
}

// CloudWatchDashboard represents a CloudWatch dashboard reference
type CloudWatchDashboard struct {
	// DashboardIdentifier is the ARN of the CloudWatch dashboard
	// +kubebuilder:validation:Pattern=`^arn:aws:cloudwatch::[0-9]{12}:dashboard/.+$`
	// +required
	DashboardIdentifier string `json:"dashboardIdentifier"`
}

// ReportOutputs defines where to store experiment reports
type ReportOutputs struct {
	// S3Configuration defines S3 settings for report output
	// +optional
	S3Configuration *S3Configuration `json:"s3Configuration,omitempty"`
}

// Tag represents a key-value pair for tagging resources
type Tag struct {
	// Key is the tag key
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +required
	Key string `json:"key"`

	// Value is the tag value
	// +kubebuilder:validation:MaxLength=256
	// +required
	Value string `json:"value"`
}

// ExperimentTemplateStatus defines the observed state of ExperimentTemplate.
type ExperimentTemplateStatus struct {
	// TemplateID is the AWS FIS experiment template ID
	// +optional
	TemplateID string `json:"templateId,omitempty"`

	// Phase represents the current phase of the experiment template
	// +kubebuilder:validation:Enum=Pending;Creating;Ready;Failed;Deleting
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`

	// LastSyncTime is the last time the template was synced with AWS FIS
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the current state of the ExperimentTemplate resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=fisexp;fistemplate
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Template ID",type=string,JSONPath=`.status.templateId`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ExperimentTemplate is the Schema for the experimenttemplates API
type ExperimentTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of ExperimentTemplate
	// +required
	Spec ExperimentTemplateSpec `json:"spec"`

	// status defines the observed state of ExperimentTemplate
	// +optional
	Status ExperimentTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ExperimentTemplateList contains a list of ExperimentTemplate
type ExperimentTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExperimentTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExperimentTemplate{}, &ExperimentTemplateList{})
}
