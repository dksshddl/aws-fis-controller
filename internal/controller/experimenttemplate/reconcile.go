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
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	fisv1alpha1 "fis.dksshddl.dev/fis-controller/api/v1alpha1"
	awsfis "fis.dksshddl.dev/fis-controller/internal/aws"
	"fis.dksshddl.dev/fis-controller/internal/utils"
)

// getRequiredParameters extracts required parameters from environment or annotations
// If roleArn is not provided, it will be automatically created
func (r *Reconciler) getRequiredParameters(ctx context.Context, template *fisv1alpha1.ExperimentTemplate) (roleArn, clusterIdentifier string, err error) {
	// Get FIS Role ARN (optional - will be auto-created if not provided)
	roleArn = os.Getenv("FIS_ROLE_ARN")
	if roleArn == "" {
		if val, ok := template.Annotations["fis.dksshddl.dev/role-arn"]; ok {
			roleArn = val
		}
	}

	// If roleArn is still empty, ensure IAM role exists (create if needed)
	if roleArn == "" {
		// Check if we already have a role in status
		if template.Status.RoleArn != "" {
			roleArn = template.Status.RoleArn
		} else {
			// Create or get existing IAM role
			createdRoleArn, err := awsfis.EnsureIAMRole(ctx, r.IAMClient, template.Namespace, template.Name, "")
			if err != nil {
				return "", "", fmt.Errorf("failed to ensure IAM role: %w", err)
			}
			roleArn = createdRoleArn
		}
	}

	// Get Cluster Identifier
	clusterIdentifier = os.Getenv("CLUSTER_IDENTIFIER")
	if clusterIdentifier == "" {
		if val, ok := template.Annotations["fis.dksshddl.dev/cluster-identifier"]; ok {
			clusterIdentifier = val
		}
	}

	// If still empty, use the cluster ARN from controller initialization
	if clusterIdentifier == "" && r.ClusterARN != "" {
		clusterIdentifier = r.ClusterARN
	}

	// If still empty, return error
	if clusterIdentifier == "" {
		return "", "", fmt.Errorf("CLUSTER_IDENTIFIER environment variable, fis.dksshddl.dev/cluster-identifier annotation, or --cluster-name flag is required")
	}

	return roleArn, clusterIdentifier, nil
}

// createFISExperimentTemplate handles the creation of AWS FIS ExperimentTemplate
func (r *Reconciler) createFISExperimentTemplate(ctx context.Context, template *fisv1alpha1.ExperimentTemplate, log logr.Logger) (ctrl.Result, error) {
	log.Info("Creating AWS FIS ExperimentTemplate")

	// Get required parameters (IAM role will be auto-created if needed)
	roleArn, clusterIdentifier, err := r.getRequiredParameters(ctx, template)
	if err != nil {
		log.Error(err, "Missing required configuration")
		return ctrl.Result{}, err
	}

	// Create Kubernetes RBAC resources (ServiceAccount, Role, RoleBinding)
	log.Info("Creating Kubernetes RBAC resources for ExperimentTemplate")
	serviceAccount, err := utils.SetupExperimentTemplateRBAC(ctx, r.Client, template.Namespace, template.Name)
	if err != nil {
		log.Error(err, "Failed to create Kubernetes RBAC resources")
		return ctrl.Result{}, err
	}
	log.Info("Successfully created Kubernetes RBAC resources", "serviceAccount", serviceAccount)

	// Create AWS FIS ExperimentTemplate
	templateID, err := r.FISClient.CreateExperimentTemplate(ctx, template, roleArn, clusterIdentifier, serviceAccount)
	if err != nil {
		log.Error(err, "Failed to create AWS FIS ExperimentTemplate")
		// Clean up RBAC resources on failure
		if cleanupErr := utils.DeleteExperimentTemplateRBAC(ctx, r.Client, template.Namespace, template.Name); cleanupErr != nil {
			log.Error(cleanupErr, "Failed to clean up RBAC resources after FIS template creation failure")
		}
		// Update status with error
		template.Status.Phase = "Failed"
		template.Status.Message = err.Error()
		if updateErr := r.Status().Update(ctx, template); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	log.Info("Successfully created AWS FIS ExperimentTemplate", "templateID", templateID, "roleArn", roleArn, "serviceAccount", serviceAccount)

	// Create EKS Access Entry for the IAM role
	if r.EKSClient != nil && r.ClusterName != "" && roleArn != "" {
		log.Info("Creating EKS Access Entry for IAM role", "roleArn", roleArn, "clusterName", r.ClusterName)

		// If role was auto-created, wait a bit for IAM propagation
		if template.Spec.AutoCreateRole {
			log.Info("Waiting for IAM role propagation before creating access entry")
			// Wait 5 seconds for IAM role to propagate
			select {
			case <-ctx.Done():
				log.Info("Context cancelled while waiting for IAM propagation")
			case <-time.After(5 * time.Second):
				log.Info("IAM role propagation wait completed")
			}
		}

		if err := awsfis.EnsureAccessEntry(ctx, r.EKSClient, r.ClusterName, roleArn); err != nil {
			log.Error(err, "Failed to create EKS Access Entry", "roleArn", roleArn, "clusterName", r.ClusterName)
			// Don't fail the creation if access entry creation fails
			// The IAM role might need more time to propagate, or the user might need to create it manually
			log.Info("Warning: EKS Access Entry creation failed. The IAM role may need more time to propagate, or you may need to create the access entry manually. You can also use aws-auth ConfigMap as an alternative.")
		} else {
			log.Info("Successfully created EKS Access Entry", "roleArn", roleArn, "clusterName", r.ClusterName)
		}
	} else {
		log.Info("Skipping EKS Access Entry creation", "hasEKSClient", r.EKSClient != nil, "hasClusterName", r.ClusterName != "", "hasRoleArn", roleArn != "")
	}

	// Update status
	template.Status.TemplateID = templateID
	template.Status.RoleArn = roleArn
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
	roleArn, clusterIdentifier, err := r.getRequiredParameters(ctx, template)
	if err != nil {
		log.Error(err, "Missing required configuration")
		return ctrl.Result{}, err
	}

	// Ensure Kubernetes RBAC resources exist (idempotent)
	log.Info("Ensuring Kubernetes RBAC resources for ExperimentTemplate")
	serviceAccount, err := utils.SetupExperimentTemplateRBAC(ctx, r.Client, template.Namespace, template.Name)
	if err != nil {
		log.Error(err, "Failed to ensure Kubernetes RBAC resources")
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

	// Ensure EKS Access Entry exists for the IAM role
	if r.EKSClient != nil && r.ClusterName != "" && roleArn != "" {
		log.Info("Ensuring EKS Access Entry for IAM role", "roleArn", roleArn, "clusterName", r.ClusterName)

		if err := awsfis.EnsureAccessEntry(ctx, r.EKSClient, r.ClusterName, roleArn); err != nil {
			log.Error(err, "Failed to ensure EKS Access Entry", "roleArn", roleArn, "clusterName", r.ClusterName)
			// Don't fail the update if access entry creation fails
			log.Info("Warning: EKS Access Entry creation failed. You may need to create the access entry manually")
		} else {
			log.Info("Successfully ensured EKS Access Entry", "roleArn", roleArn, "clusterName", r.ClusterName)
		}
	}

	// Update status
	template.Status.RoleArn = roleArn
	template.Status.Phase = "Ready"
	template.Status.Message = "AWS FIS ExperimentTemplate updated successfully"
	template.Status.ObservedGeneration = template.Generation
	if err := r.Status().Update(ctx, template); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// handleDeletion handles the deletion of AWS FIS ExperimentTemplate, IAM Role, and Kubernetes RBAC
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

	// Delete IAM Role if it was auto-created (check if RoleArn is in status)
	if template.Status.RoleArn != "" {
		// Only delete if it's an auto-created role (follows our naming pattern)
		if err := awsfis.DeleteIAMRole(ctx, r.IAMClient, template.Namespace, template.Name); err != nil {
			log.Error(err, "Failed to delete IAM role")
			// Don't fail the deletion if IAM role deletion fails
			// Just log the error and continue
		} else {
			log.Info("Successfully deleted IAM role", "roleArn", template.Status.RoleArn)
		}
	}

	// Delete Kubernetes RBAC resources
	log.Info("Deleting Kubernetes RBAC resources for ExperimentTemplate")
	if err := utils.DeleteExperimentTemplateRBAC(ctx, r.Client, template.Namespace, template.Name); err != nil {
		log.Error(err, "Failed to delete Kubernetes RBAC resources")
		// Don't fail the deletion if RBAC cleanup fails
		// Just log the error and continue
	} else {
		log.Info("Successfully deleted Kubernetes RBAC resources")
	}

	return ctrl.Result{}, nil
}
