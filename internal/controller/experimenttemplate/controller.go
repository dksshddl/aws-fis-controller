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

package experimenttemplate

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fisv1alpha1 "fis.dksshddl.dev/fis-controller/api/v1alpha1"
	awsfis "fis.dksshddl.dev/fis-controller/internal/aws"
)

const (
	finalizerName = "fis.fis.dksshddl.dev/finalizer"
)

// Reconciler reconciles a ExperimentTemplate object
type Reconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	FISClient   *awsfis.FISClient
	IAMClient   *awsfis.IAMClient
	EKSClient   *awsfis.EKSClient
	ClusterARN  string
	ClusterName string
}

// +kubebuilder:rbac:groups=fis.fis.dksshddl.dev,resources=experimenttemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fis.fis.dksshddl.dev,resources=experimenttemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fis.fis.dksshddl.dev,resources=experimenttemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the ExperimentTemplate instance
	experimentTemplate := &fisv1alpha1.ExperimentTemplate{}
	if err := r.Get(ctx, req.NamespacedName, experimentTemplate); err != nil {
		if errors.IsNotFound(err) {
			log.Info("ExperimentTemplate resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ExperimentTemplate")
		return ctrl.Result{}, err
	}

	log.Info("Reconciling ExperimentTemplate", "name", experimentTemplate.Name, "namespace", experimentTemplate.Namespace)

	// Initialize FIS client if not already initialized
	if r.FISClient == nil {
		fisClient, err := awsfis.NewFISClient(ctx, awsfis.FISConfig{
			Region:     "ap-northeast-2",
			MaxRetries: 3,
		})
		if err != nil {
			log.Error(err, "Failed to create FIS client")
			return ctrl.Result{}, err
		}
		r.FISClient = fisClient
	}

	// Handle deletion
	if !experimentTemplate.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(experimentTemplate, finalizerName) {
			// Delete AWS FIS ExperimentTemplate
			if result, err := r.handleDeletion(ctx, experimentTemplate, log); err != nil {
				return result, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(experimentTemplate, finalizerName)
			if err := r.Update(ctx, experimentTemplate); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(experimentTemplate, finalizerName) {
		controllerutil.AddFinalizer(experimentTemplate, finalizerName)
		if err := r.Update(ctx, experimentTemplate); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if AWS FIS ExperimentTemplate already exists
	if experimentTemplate.Status.TemplateID != "" {
		log.Info("AWS FIS ExperimentTemplate already exists", "templateID", experimentTemplate.Status.TemplateID)

		// Check if spec has changed (compare generation with observedGeneration)
		if experimentTemplate.Generation != experimentTemplate.Status.ObservedGeneration {
			log.Info("ExperimentTemplate spec has changed, updating AWS FIS ExperimentTemplate")
			return r.updateFISExperimentTemplate(ctx, experimentTemplate, log)
		}

		// No changes, nothing to do
		return ctrl.Result{}, nil
	}

	// Create AWS FIS ExperimentTemplate
	return r.createFISExperimentTemplate(ctx, experimentTemplate, log)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fisv1alpha1.ExperimentTemplate{}).
		Named("experimenttemplate").
		Complete(r)
}
