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
	"fmt"
	"os"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	fisv1alpha1 "fis.dksshddl.dev/fis-controller/api/v1alpha1"
)

// getRequiredParameters extracts required parameters from environment or annotations
func (r *Reconciler) getRequiredParameters(template *fisv1alpha1.ExperimentTemplate) (roleArn, clusterIdentifier, serviceAccount string, err error) {
	// Get FIS Role ARN
	roleArn = os.Getenv("FIS_ROLE_ARN")
	if roleArn == "" {
		if val, ok := template.Annotations["fis.dksshddl.dev/role-arn"]; ok {
			roleArn = val
		} else {
			return "", "", "", fmt.Errorf("FIS_ROLE_ARN environment variable or fis.dksshddl.dev/role-arn annotation is required")
		}
	}

	// Get Cluster Identifier
	clusterIdentifier = os.Getenv("CLUSTER_IDENTIFIER")
	if clusterIdentifier == "" {
		if val, ok := template.Annotations["fis.dksshddl.dev/cluster-identifier"]; ok {
			clusterIdentifier = val
		} else {
			return "", "", "", fmt.Errorf("CLUSTER_IDENTIFIER environment variable or fis.dksshddl.dev/cluster-identifier annotation is required")
		}
	}

	// Get Service Account (optional, has default)
	serviceAccount = "fis-pod-sa"
	if val, ok := template.Annotations["fis.dksshddl.dev/service-account"]; ok {
		serviceAccount = val
	}

	return roleArn, clusterIdentifier, serviceAccount, nil
}

// createFISExperimentTemplate handles the creation of AWS FIS ExperimentTemplate
func (r *Reconciler) createFISExperimentTemplate(ctx context.Context, template *fisv1alpha1.ExperimentTemplate, log logr.Logger) (ctrl.Result, error) {
	log.Info("Creating AWS FIS ExperimentTemplate")

	// Get required parameters
	roleArn, clusterIdentifier, serviceAccount, err := r.getRequiredParameters(template)
	if err != nil {
		log.Error(err, "Missing required configuration")
		return ctrl.Result{}, err
	}

	// Create AWS FIS ExperimentTemplate
	templateID, err := r.FISClient.CreateExperimentTemplate(ctx, template, roleArn, clusterIdentifier, serviceAccount)
	if err != nil {
		log.Error(err, "Failed to create AWS FIS ExperimentTemplate")
		// Update status with error
		template.Status.Phase = "Failed"
		template.Status.Message = err.Error()
		if updateErr := r.Status().Update(ctx, template); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	log.Info("Successfully created AWS FIS ExperimentTemplate", "templateID", templateID)

	// Update status
	template.Status.TemplateID = templateID
	template.Status.Phase = "Ready"
	template.Status.Message = "AWS FIS ExperimentTemplate created successfully"
	template.Status.ObservedGeneration = template.Generation
	if err := r.Status().Update(ctx, template); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateFISExperimentTemplate handles the update of AWS FIS ExperimentTemplate
func (r *Reconciler) updateFISExperimentTemplate(ctx context.Context, template *fisv1alpha1.ExperimentTemplate, log logr.Logger) (ctrl.Result, error) {
	log.Info("Updating AWS FIS ExperimentTemplate", "templateID", template.Status.TemplateID)

	// Get required parameters
	roleArn, clusterIdentifier, serviceAccount, err := r.getRequiredParameters(template)
	if err != nil {
		log.Error(err, "Missing required configuration")
		return ctrl.Result{}, err
	}

	// Update AWS FIS ExperimentTemplate
	if err := r.FISClient.UpdateExperimentTemplate(ctx, template, template.Status.TemplateID, roleArn, clusterIdentifier, serviceAccount); err != nil {
		log.Error(err, "Failed to update AWS FIS ExperimentTemplate")
		// Update status with error
		template.Status.Phase = "Failed"
		template.Status.Message = err.Error()
		if updateErr := r.Status().Update(ctx, template); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	log.Info("Successfully updated AWS FIS ExperimentTemplate", "templateID", template.Status.TemplateID)

	// Update status
	template.Status.Phase = "Ready"
	template.Status.Message = "AWS FIS ExperimentTemplate updated successfully"
	template.Status.ObservedGeneration = template.Generation
	if err := r.Status().Update(ctx, template); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// handleDeletion handles the deletion of AWS FIS ExperimentTemplate
func (r *Reconciler) handleDeletion(ctx context.Context, template *fisv1alpha1.ExperimentTemplate, log logr.Logger) (ctrl.Result, error) {
	log.Info("Deleting AWS FIS ExperimentTemplate", "templateID", template.Status.TemplateID)

	// Delete AWS FIS ExperimentTemplate if it exists
	if template.Status.TemplateID != "" {
		if err := r.FISClient.DeleteExperimentTemplate(ctx, template.Status.TemplateID); err != nil {
			log.Error(err, "Failed to delete AWS FIS ExperimentTemplate")
			return ctrl.Result{}, err
		}
		log.Info("Successfully deleted AWS FIS ExperimentTemplate", "templateID", template.Status.TemplateID)
	}

	return ctrl.Result{}, nil
}
