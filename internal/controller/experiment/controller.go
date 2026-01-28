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

package experiment

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	fisv1alpha1 "fis.dksshddl.dev/fis-controller/api/v1alpha1"
	awsfis "fis.dksshddl.dev/fis-controller/internal/aws"
)

const (
	experimentFinalizer = "fis.dksshddl.dev/experiment-finalizer"
)

// Reconciler reconciles a Experiment object
type Reconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	FISClient *awsfis.FISClient
}

// +kubebuilder:rbac:groups=fis.fis.dksshddl.dev,resources=experiments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fis.fis.dksshddl.dev,resources=experiments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fis.fis.dksshddl.dev,resources=experiments/finalizers,verbs=update
// +kubebuilder:rbac:groups=fis.fis.dksshddl.dev,resources=experimenttemplates,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Experiment instance
	experiment := &fisv1alpha1.Experiment{}
	if err := r.Get(ctx, req.NamespacedName, experiment); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Experiment")
		return ctrl.Result{}, err
	}

	log.Info("Reconciling Experiment", "name", experiment.Name, "namespace", experiment.Namespace)

	// Handle deletion
	if !experiment.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, experiment, log)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(experiment, experimentFinalizer) {
		controllerutil.AddFinalizer(experiment, experimentFinalizer)
		if err := r.Update(ctx, experiment); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if suspended
	if experiment.Spec.Suspend != nil && *experiment.Spec.Suspend {
		log.Info("Experiment is suspended, skipping")
		return ctrl.Result{}, nil
	}

	// Resolve template ID
	templateID, err := r.resolveTemplateID(ctx, experiment, log)
	if err != nil {
		log.Error(err, "Failed to resolve template ID")
		experiment.Status.State = "failed"
		experiment.Status.Reason = fmt.Sprintf("Failed to resolve template ID: %v", err)
		if updateErr := r.Status().Update(ctx, experiment); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	// Update status with resolved template ID
	if experiment.Status.TemplateID != templateID {
		experiment.Status.TemplateID = templateID
		if err := r.Status().Update(ctx, experiment); err != nil {
			log.Error(err, "Failed to update template ID in status")
			return ctrl.Result{}, err
		}
	}

	// Handle scheduled vs one-time experiments
	if experiment.Spec.Schedule != "" {
		return r.handleScheduledExperiment(ctx, experiment, log)
	}

	// One-time experiment (Job mode)
	return r.handleOneTimeExperiment(ctx, experiment, log)
}

// resolveTemplateID resolves the template ID from spec
func (r *Reconciler) resolveTemplateID(ctx context.Context, experiment *fisv1alpha1.Experiment, log logr.Logger) (string, error) {
	// If ID is provided, use it directly
	if experiment.Spec.ExperimentTemplate.ID != "" {
		return experiment.Spec.ExperimentTemplate.ID, nil
	}

	// If Name is provided, look up the ExperimentTemplate CRD
	// ExperimentTemplate is cluster-scoped, so no namespace needed
	if experiment.Spec.ExperimentTemplate.Name != "" {
		template := &fisv1alpha1.ExperimentTemplate{}
		namespacedName := types.NamespacedName{
			Name: experiment.Spec.ExperimentTemplate.Name,
		}
		if err := r.Get(ctx, namespacedName, template); err != nil {
			return "", fmt.Errorf("failed to get ExperimentTemplate %s: %w", experiment.Spec.ExperimentTemplate.Name, err)
		}

		if template.Status.TemplateID == "" {
			return "", fmt.Errorf("ExperimentTemplate %s does not have a template ID yet", experiment.Spec.ExperimentTemplate.Name)
		}

		return template.Status.TemplateID, nil
	}

	return "", fmt.Errorf("either experimentTemplate.id or experimentTemplate.name must be specified")
}

// handleOneTimeExperiment handles one-time experiment execution (Job mode)
func (r *Reconciler) handleOneTimeExperiment(ctx context.Context, experiment *fisv1alpha1.Experiment, log logr.Logger) (ctrl.Result, error) {
	// If experiment hasn't been started yet, start it
	if experiment.Status.ExperimentID == "" {
		return r.startExperiment(ctx, experiment, log)
	}

	// If experiment is already started, sync its state
	return r.syncExperimentState(ctx, experiment, log)
}

// handleScheduledExperiment handles scheduled experiment execution (CronJob mode)
func (r *Reconciler) handleScheduledExperiment(ctx context.Context, experiment *fisv1alpha1.Experiment, log logr.Logger) (ctrl.Result, error) {
	// Parse cron schedule
	schedule, err := cron.ParseStandard(experiment.Spec.Schedule)
	if err != nil {
		log.Error(err, "Invalid cron schedule", "schedule", experiment.Spec.Schedule)
		experiment.Status.State = "failed"
		experiment.Status.Reason = fmt.Sprintf("Invalid cron schedule: %v", err)
		if updateErr := r.Status().Update(ctx, experiment); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	now := time.Now()

	// Determine if we should run now based on LastScheduleTime
	shouldRun := false
	var missedRun *time.Time

	if experiment.Status.LastScheduleTime == nil {
		// Never run before - check if there's a schedule time that has passed
		// For first run, we start from creation time
		creationTime := experiment.CreationTimestamp.Time
		nextAfterCreation := schedule.Next(creationTime)
		if !nextAfterCreation.After(now) {
			shouldRun = true
			missedRun = &nextAfterCreation
		}
	} else {
		// Check if we missed any scheduled runs since last execution
		nextAfterLast := schedule.Next(experiment.Status.LastScheduleTime.Time)
		if !nextAfterLast.After(now) {
			shouldRun = true
			missedRun = &nextAfterLast
		}
	}

	// Calculate next schedule time for status
	var nextScheduleTime time.Time
	if shouldRun && missedRun != nil {
		nextScheduleTime = schedule.Next(*missedRun)
	} else if experiment.Status.LastScheduleTime != nil {
		nextScheduleTime = schedule.Next(experiment.Status.LastScheduleTime.Time)
		if !nextScheduleTime.After(now) {
			nextScheduleTime = schedule.Next(now)
		}
	} else {
		nextScheduleTime = schedule.Next(now)
	}

	// Update next schedule time in status (don't return, continue processing)
	nextScheduleTimeMeta := metav1.NewTime(nextScheduleTime)
	statusChanged := false
	if experiment.Status.NextScheduleTime == nil || !experiment.Status.NextScheduleTime.Equal(&nextScheduleTimeMeta) {
		experiment.Status.NextScheduleTime = &nextScheduleTimeMeta
		statusChanged = true
	}

	if !shouldRun {
		// Not time yet, update status if needed and requeue
		if statusChanged {
			if err := r.Status().Update(ctx, experiment); err != nil {
				log.Error(err, "Failed to update next schedule time")
				return ctrl.Result{}, err
			}
		}
		requeueAfter := nextScheduleTime.Sub(now)
		log.Info("Experiment scheduled", "nextRun", nextScheduleTime, "requeueAfter", requeueAfter)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Time to run the experiment
	log.Info("Starting scheduled experiment", "schedule", experiment.Spec.Schedule, "missedRun", missedRun)

	// Start the experiment
	result, err := r.startExperiment(ctx, experiment, log)
	if err != nil {
		return result, err
	}

	// Update last schedule time
	lastScheduleTime := metav1.Now()
	experiment.Status.LastScheduleTime = &lastScheduleTime
	experiment.Status.NextScheduleTime = &nextScheduleTimeMeta
	if err := r.Status().Update(ctx, experiment); err != nil {
		log.Error(err, "Failed to update schedule times")
		return ctrl.Result{}, err
	}

	// Requeue for next schedule
	requeueAfter := nextScheduleTime.Sub(now)
	log.Info("Scheduled experiment started, waiting for next schedule", "nextRun", nextScheduleTime)

	// Clean up old experiments based on history limits
	if err := r.cleanupExperimentHistory(ctx, experiment, log); err != nil {
		log.Error(err, "Failed to cleanup experiment history")
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// startExperiment starts a new AWS FIS experiment
func (r *Reconciler) startExperiment(ctx context.Context, experiment *fisv1alpha1.Experiment, log logr.Logger) (ctrl.Result, error) {
	log.Info("Starting AWS FIS Experiment", "templateID", experiment.Status.TemplateID)

	// Start the experiment
	experimentID, err := r.FISClient.StartExperiment(ctx, experiment)
	if err != nil {
		log.Error(err, "Failed to start AWS FIS Experiment")
		// Update status with error
		experiment.Status.State = "failed"
		experiment.Status.Reason = err.Error()
		if updateErr := r.Status().Update(ctx, experiment); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	log.Info("Successfully started AWS FIS Experiment", "experimentID", experimentID)

	// Update status
	experiment.Status.ExperimentID = experimentID
	experiment.Status.State = "initiating"
	experiment.Status.Reason = "Experiment is initiating"
	now := metav1.Now()
	experiment.Status.StartTime = &now
	experiment.Status.Active = 1

	if err := r.Status().Update(ctx, experiment); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// For one-time experiments, requeue to check status
	// For scheduled experiments, this will be handled by the schedule
	if experiment.Spec.Schedule == "" {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// syncExperimentState syncs the experiment state from AWS
func (r *Reconciler) syncExperimentState(ctx context.Context, experiment *fisv1alpha1.Experiment, log logr.Logger) (ctrl.Result, error) {
	log.Info("Syncing experiment state", "experimentID", experiment.Status.ExperimentID)

	// Get current experiment state from AWS
	awsExperiment, err := r.FISClient.GetExperiment(ctx, experiment.Status.ExperimentID)
	if err != nil {
		log.Error(err, "Failed to get experiment state from AWS")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Update status from AWS state
	previousState := experiment.Status.State
	experiment.Status.State = string(awsExperiment.State.Status)
	if awsExperiment.State.Reason != nil {
		experiment.Status.Reason = *awsExperiment.State.Reason
	}

	// Update timestamps
	if awsExperiment.StartTime != nil && experiment.Status.StartTime == nil {
		startTime := metav1.NewTime(*awsExperiment.StartTime)
		experiment.Status.StartTime = &startTime
	}
	if awsExperiment.EndTime != nil {
		endTime := metav1.NewTime(*awsExperiment.EndTime)
		experiment.Status.EndTime = &endTime
		experiment.Status.Active = 0
	} else {
		experiment.Status.Active = 1
	}

	// Update target account configurations count
	if awsExperiment.TargetAccountConfigurationsCount != nil {
		experiment.Status.TargetAccountConfigurationsCount = *awsExperiment.TargetAccountConfigurationsCount
	}

	if err := r.Status().Update(ctx, experiment); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Log state changes
	if previousState != experiment.Status.State {
		log.Info("Experiment state changed",
			"previousState", previousState,
			"newState", experiment.Status.State,
			"reason", experiment.Status.Reason)
	}

	// Determine requeue behavior based on state
	switch experiment.Status.State {
	case "initiating", "pending", "running", "stopping":
		// Still in progress, check again soon
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	case "completed", "stopped", "failed":
		// Terminal state, no need to requeue
		log.Info("Experiment reached terminal state", "state", experiment.Status.State)
		return ctrl.Result{}, nil
	default:
		// Unknown state, check again later
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
}

// handleDeletion handles the deletion of an Experiment
func (r *Reconciler) handleDeletion(ctx context.Context, experiment *fisv1alpha1.Experiment, log logr.Logger) (ctrl.Result, error) {
	log.Info("Handling Experiment deletion", "experimentID", experiment.Status.ExperimentID)

	// If experiment is running, stop it
	if experiment.Status.ExperimentID != "" {
		state := experiment.Status.State
		if state == "initiating" || state == "pending" || state == "running" {
			log.Info("Stopping running experiment", "experimentID", experiment.Status.ExperimentID)
			if err := r.FISClient.StopExperiment(ctx, experiment.Status.ExperimentID); err != nil {
				log.Error(err, "Failed to stop experiment")
				// Don't fail deletion if stop fails
			} else {
				log.Info("Successfully stopped experiment", "experimentID", experiment.Status.ExperimentID)
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(experiment, experimentFinalizer)
	if err := r.Update(ctx, experiment); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fisv1alpha1.Experiment{}).
		Named("experiment").
		Complete(r)
}

// cleanupExperimentHistory cleans up old AWS FIS experiments based on history limits
func (r *Reconciler) cleanupExperimentHistory(ctx context.Context, experiment *fisv1alpha1.Experiment, log logr.Logger) error {
	// Only cleanup for scheduled experiments
	if experiment.Spec.Schedule == "" {
		return nil
	}

	templateID := experiment.Status.TemplateID
	if templateID == "" {
		return nil
	}

	// Get history limits with defaults
	successLimit := int32(3)
	failedLimit := int32(1)
	if experiment.Spec.SuccessfulExperimentsHistoryLimit != nil {
		successLimit = *experiment.Spec.SuccessfulExperimentsHistoryLimit
	}
	if experiment.Spec.FailedExperimentsHistoryLimit != nil {
		failedLimit = *experiment.Spec.FailedExperimentsHistoryLimit
	}

	// List all experiments for this template
	experiments, err := r.FISClient.ListExperimentsByTemplate(ctx, templateID)
	if err != nil {
		return fmt.Errorf("failed to list experiments: %w", err)
	}

	// Separate by state
	var successful, failed []awsfis.ExperimentSummary
	for _, exp := range experiments {
		switch exp.State {
		case "completed":
			successful = append(successful, exp)
		case "failed", "stopped":
			failed = append(failed, exp)
		}
	}

	// Sort by start time (newest first) and cleanup excess
	sortByStartTimeDesc(successful)
	sortByStartTimeDesc(failed)

	// Log cleanup info
	if len(successful) > int(successLimit) || len(failed) > int(failedLimit) {
		log.Info("Cleaning up experiment history",
			"successfulCount", len(successful),
			"successfulLimit", successLimit,
			"failedCount", len(failed),
			"failedLimit", failedLimit)
	}

	// Note: AWS FIS doesn't provide DeleteExperiment API
	// Experiments are automatically retained for 120 days
	// We log which experiments would be cleaned up for visibility
	if len(successful) > int(successLimit) {
		toCleanup := successful[successLimit:]
		for _, exp := range toCleanup {
			log.Info("Experiment exceeds history limit (auto-cleanup by AWS after 120 days)",
				"experimentID", exp.ID,
				"state", exp.State,
				"startTime", exp.StartTime)
		}
	}

	if len(failed) > int(failedLimit) {
		toCleanup := failed[failedLimit:]
		for _, exp := range toCleanup {
			log.Info("Experiment exceeds history limit (auto-cleanup by AWS after 120 days)",
				"experimentID", exp.ID,
				"state", exp.State,
				"startTime", exp.StartTime)
		}
	}

	return nil
}

// sortByStartTimeDesc sorts experiments by start time in descending order (newest first)
func sortByStartTimeDesc(experiments []awsfis.ExperimentSummary) {
	for i := 0; i < len(experiments)-1; i++ {
		for j := i + 1; j < len(experiments); j++ {
			iTime := experiments[i].StartTime
			jTime := experiments[j].StartTime
			if iTime == nil || (jTime != nil && jTime.After(*iTime)) {
				experiments[i], experiments[j] = experiments[j], experiments[i]
			}
		}
	}
}
