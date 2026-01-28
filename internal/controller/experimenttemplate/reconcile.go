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
			// Create or get existing IAM role (cluster-scoped, no namespace)
			createdRoleArn, err := awsfis.EnsureIAMRole(ctx, r.IAMClient, "", template.Name, "")
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

// getTargetNamespaces extracts unique namespaces from targets
func getTargetNamespaces(template *fisv1alpha1.ExperimentTemplate) []string {
	namespaceSet := make(map[string]bool)
	for _, target := range template.Spec.Targets {
		if target.Namespace != "" {
			namespaceSet[target.Namespace] = true
		}
	}
	namespaces := make([]string, 0, len(namespaceSet))
	for ns := range namespaceSet {
		namespaces = append(namespaces, ns)
	}
	return namespaces
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

	// Get target namespaces from targets
	targetNamespaces := getTargetNamespaces(template)
	if len(targetNamespaces) == 0 {
		return ctrl.Result{}, fmt.Errorf("no target namespaces found in targets")
	}

	// Create Kubernetes RBAC resources in each target namespace
	log.Info("Creating Kubernetes RBAC resources for ExperimentTemplate", "namespaces", targetNamespaces)
	var serviceAccount string
	for _, ns := range targetNamespaces {
		sa, err := utils.SetupExperimentTemplateRBAC(ctx, r.Client, ns, template.Name)
		if err != nil {
			log.Error(err, "Failed to create Kubernetes RBAC resources", "namespace", ns)
			return ctrl.Result{}, err
		}
		serviceAccount = sa // Use the same service account name pattern
	}
	log.Info("Successfully created Kubernetes RBAC resources", "serviceAccount", serviceAccount)

	// Create AWS FIS ExperimentTemplate
	templateID, err := r.FISClient.CreateExperimentTemplate(ctx, template, roleArn, clusterIdentifier, serviceAccount)
	if err != nil {
		log.Error(err, "Failed to create AWS FIS ExperimentTemplate")
		// Clean up RBAC resources on failure
		for _, ns := range targetNamespaces {
			if cleanupErr := utils.DeleteExperimentTemplateRBAC(ctx, r.Client, ns, template.Name); cleanupErr != nil {
				log.Error(cleanupErr, "Failed to clean up RBAC resources after FIS template creation failure", "namespace", ns)
			}
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
	// Username format: fis-{templateName} (matches RoleBinding subject)
	username := fmt.Sprintf("fis-%s", template.Name)
	if r.EKSClient != nil && r.ClusterName != "" && roleArn != "" {
		log.Info("Creating EKS Access Entry for IAM role", "roleArn", roleArn, "clusterName", r.ClusterName, "username", username)

		// If role was auto-created, wait for IAM propagation and retry
		if template.Spec.AutoCreateRole {
			log.Info("Waiting for IAM role propagation before creating access entry")

			// Retry up to 3 times with increasing wait times
			var accessEntryErr error
			contextCancelled := false
			for attempt := 1; attempt <= 3; attempt++ {
				waitTime := time.Duration(attempt*5) * time.Second
				log.Info("Waiting for IAM role propagation", "attempt", attempt, "waitTime", waitTime)

				select {
				case <-ctx.Done():
					log.Info("Context cancelled while waiting for IAM propagation")
					contextCancelled = true
				case <-time.After(waitTime):
				}

				if contextCancelled {
					break
				}

				accessEntryErr = awsfis.EnsureAccessEntry(ctx, r.EKSClient, r.ClusterName, roleArn, username)
				if accessEntryErr == nil {
					log.Info("Successfully created EKS Access Entry", "roleArn", roleArn, "clusterName", r.ClusterName, "username", username, "attempt", attempt)
					break
				}
				log.Info("Access Entry creation attempt failed, will retry", "attempt", attempt, "error", accessEntryErr.Error())
			}

			if accessEntryErr != nil {
				log.Error(accessEntryErr, "Failed to create EKS Access Entry after retries", "roleArn", roleArn, "clusterName", r.ClusterName)
				log.Info("Warning: EKS Access Entry creation failed. You may need to create the access entry manually using: aws eks create-access-entry --cluster-name " + r.ClusterName + " --principal-arn " + roleArn + " --username " + username)
			}
		} else {
			// For user-provided roles, try once without waiting
			if err := awsfis.EnsureAccessEntry(ctx, r.EKSClient, r.ClusterName, roleArn, username); err != nil {
				log.Error(err, "Failed to create EKS Access Entry", "roleArn", roleArn, "clusterName", r.ClusterName)
				log.Info("Warning: EKS Access Entry creation failed. You may need to create the access entry manually.")
			} else {
				log.Info("Successfully created EKS Access Entry", "roleArn", roleArn, "clusterName", r.ClusterName, "username", username)
			}
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

	// Get target namespaces from targets
	targetNamespaces := getTargetNamespaces(template)
	if len(targetNamespaces) == 0 {
		return ctrl.Result{}, fmt.Errorf("no target namespaces found in targets")
	}

	// Ensure Kubernetes RBAC resources exist in each target namespace (idempotent)
	log.Info("Ensuring Kubernetes RBAC resources for ExperimentTemplate", "namespaces", targetNamespaces)
	var serviceAccount string
	for _, ns := range targetNamespaces {
		sa, err := utils.SetupExperimentTemplateRBAC(ctx, r.Client, ns, template.Name)
		if err != nil {
			log.Error(err, "Failed to ensure Kubernetes RBAC resources", "namespace", ns)
			return ctrl.Result{}, err
		}
		serviceAccount = sa
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
	username := fmt.Sprintf("fis-%s", template.Name)
	if r.EKSClient != nil && r.ClusterName != "" && roleArn != "" {
		log.Info("Ensuring EKS Access Entry for IAM role", "roleArn", roleArn, "clusterName", r.ClusterName, "username", username)

		if err := awsfis.EnsureAccessEntry(ctx, r.EKSClient, r.ClusterName, roleArn, username); err != nil {
			log.Error(err, "Failed to ensure EKS Access Entry", "roleArn", roleArn, "clusterName", r.ClusterName)
			// Don't fail the update if access entry creation fails
			log.Info("Warning: EKS Access Entry creation failed. You may need to create the access entry manually")
		} else {
			log.Info("Successfully ensured EKS Access Entry", "roleArn", roleArn, "clusterName", r.ClusterName, "username", username)
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

	// Delete EKS Access Entry if it exists
	if r.EKSClient != nil && r.ClusterName != "" && template.Status.RoleArn != "" {
		log.Info("Deleting EKS Access Entry", "roleArn", template.Status.RoleArn, "clusterName", r.ClusterName)
		if err := awsfis.DeleteAccessEntryIfExists(ctx, r.EKSClient, r.ClusterName, template.Status.RoleArn); err != nil {
			log.Error(err, "Failed to delete EKS Access Entry")
			// Don't fail the deletion if access entry deletion fails
			// Just log the error and continue
		} else {
			log.Info("Successfully deleted EKS Access Entry", "roleArn", template.Status.RoleArn)
		}
	}

	// Delete IAM Role if it was auto-created (check if RoleArn is in status)
	if template.Status.RoleArn != "" {
		// Only delete if it's an auto-created role (follows our naming pattern)
		if err := awsfis.DeleteIAMRole(ctx, r.IAMClient, "", template.Name); err != nil {
			log.Error(err, "Failed to delete IAM role")
			// Don't fail the deletion if IAM role deletion fails
			// Just log the error and continue
		} else {
			log.Info("Successfully deleted IAM role", "roleArn", template.Status.RoleArn)
		}
	}

	// Delete Kubernetes RBAC resources from all target namespaces
	targetNamespaces := getTargetNamespaces(template)
	log.Info("Deleting Kubernetes RBAC resources for ExperimentTemplate", "namespaces", targetNamespaces)
	for _, ns := range targetNamespaces {
		if err := utils.DeleteExperimentTemplateRBAC(ctx, r.Client, ns, template.Name); err != nil {
			log.Error(err, "Failed to delete Kubernetes RBAC resources", "namespace", ns)
			// Don't fail the deletion if RBAC cleanup fails
			// Just log the error and continue
		} else {
			log.Info("Successfully deleted Kubernetes RBAC resources", "namespace", ns)
		}
	}

	return ctrl.Result{}, nil
}
