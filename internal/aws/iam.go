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

package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// IAMClient wraps AWS IAM client
type IAMClient struct {
	client *iam.Client
}

// NewIAMClient creates a new IAM client using the same config as FIS client
func NewIAMClient(awsConfig aws.Config) *IAMClient {
	return &IAMClient{
		client: iam.NewFromConfig(awsConfig),
	}
}

// CreateFISRole creates an IAM role for FIS experiment template
func (c *IAMClient) CreateFISRole(ctx context.Context, roleName, namespace, templateName string) (string, error) {
	// Trust policy for FIS service
	trustPolicy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Principal": map[string]string{
					"Service": "fis.amazonaws.com",
				},
				"Action": "sts:AssumeRole",
			},
		},
	}

	trustPolicyJSON, err := json.Marshal(trustPolicy)
	if err != nil {
		return "", fmt.Errorf("failed to marshal trust policy: %w", err)
	}

	// Create role
	createRoleInput := &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(string(trustPolicyJSON)),
		Description:              aws.String(fmt.Sprintf("IAM role for FIS experiment template %s/%s", namespace, templateName)),
		Tags: []iamtypes.Tag{
			{
				Key:   aws.String("ManagedBy"),
				Value: aws.String("aws-fis-controller"),
			},
			{
				Key:   aws.String("kubernetes.io/name"),
				Value: aws.String(templateName),
			},
			{
				Key:   aws.String("kubernetes.io/namespace"),
				Value: aws.String(namespace),
			},
		},
	}

	createRoleOutput, err := c.client.CreateRole(ctx, createRoleInput)
	if err != nil {
		return "", fmt.Errorf("failed to create IAM role: %w", err)
	}

	roleArn := aws.ToString(createRoleOutput.Role.Arn)

	// Attach FIS service policy
	// This policy allows FIS to perform actions on EKS pods
	policyDocument := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"eks:DescribeCluster",
					"eks:ListClusters",
				},
				"Resource": "*",
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"iam:PassRole",
				},
				"Resource": "*",
				"Condition": map[string]interface{}{
					"StringEquals": map[string]string{
						"iam:PassedToService": "fis.amazonaws.com",
					},
				},
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"logs:CreateLogDelivery",
					"logs:PutResourcePolicy",
					"logs:DescribeResourcePolicies",
					"logs:DescribeLogGroups",
				},
				"Resource": "*",
			},
		},
	}

	policyDocumentJSON, err := json.Marshal(policyDocument)
	if err != nil {
		return "", fmt.Errorf("failed to marshal policy document: %w", err)
	}

	policyName := fmt.Sprintf("%s-policy", roleName)
	putPolicyInput := &iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(string(policyDocumentJSON)),
	}

	_, err = c.client.PutRolePolicy(ctx, putPolicyInput)
	if err != nil {
		return "", fmt.Errorf("failed to attach policy to role: %w", err)
	}

	return roleArn, nil
}

// DeleteFISRole deletes an IAM role created for FIS experiment template
func (c *IAMClient) DeleteFISRole(ctx context.Context, roleName string) error {
	// List and delete all inline policies
	listPoliciesInput := &iam.ListRolePoliciesInput{
		RoleName: aws.String(roleName),
	}

	listPoliciesOutput, err := c.client.ListRolePolicies(ctx, listPoliciesInput)
	if err != nil {
		return fmt.Errorf("failed to list role policies: %w", err)
	}

	for _, policyName := range listPoliciesOutput.PolicyNames {
		deletePolicyInput := &iam.DeleteRolePolicyInput{
			RoleName:   aws.String(roleName),
			PolicyName: aws.String(policyName),
		}

		_, err := c.client.DeleteRolePolicy(ctx, deletePolicyInput)
		if err != nil {
			return fmt.Errorf("failed to delete role policy %s: %w", policyName, err)
		}
	}

	// Delete the role
	deleteRoleInput := &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	}

	_, err = c.client.DeleteRole(ctx, deleteRoleInput)
	if err != nil {
		return fmt.Errorf("failed to delete IAM role: %w", err)
	}

	return nil
}

// RoleExists checks if an IAM role exists
func (c *IAMClient) RoleExists(ctx context.Context, roleName string) (bool, error) {
	getRoleInput := &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	}

	_, err := c.client.GetRole(ctx, getRoleInput)
	if err != nil {
		// Check if it's a NoSuchEntity error
		var noSuchEntityErr *iamtypes.NoSuchEntityException
		if errors.As(err, &noSuchEntityErr) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get IAM role: %w", err)
	}

	return true, nil
}

// GenerateRoleName generates a unique role name for an experiment template
func GenerateRoleName(namespace, templateName string) string {
	// IAM role names must be alphanumeric plus +=,.@-_ and max 64 chars
	// Format: fis-<namespace>-<templateName>
	roleName := fmt.Sprintf("fis-%s-%s", namespace, templateName)

	// Truncate if too long (max 64 chars)
	if len(roleName) > 64 {
		roleName = roleName[:64]
	}

	return roleName
}

// EnsureIAMRole ensures an IAM role exists for the experiment template
// If roleArn is provided, it validates the role exists
// If roleArn is empty, it creates a new role
func EnsureIAMRole(ctx context.Context, iamClient *IAMClient, namespace, templateName, roleArn string) (string, error) {
	// If roleArn is provided, just return it (assume it's valid)
	if roleArn != "" {
		return roleArn, nil
	}

	// Generate role name
	roleName := GenerateRoleName(namespace, templateName)

	// Check if role already exists
	exists, err := iamClient.RoleExists(ctx, roleName)
	if err != nil {
		return "", fmt.Errorf("failed to check if role exists: %w", err)
	}

	if exists {
		// Role already exists, get its ARN
		getRoleInput := &iam.GetRoleInput{
			RoleName: aws.String(roleName),
		}
		getRoleOutput, err := iamClient.client.GetRole(ctx, getRoleInput)
		if err != nil {
			return "", fmt.Errorf("failed to get existing role: %w", err)
		}
		return aws.ToString(getRoleOutput.Role.Arn), nil
	}

	// Create new role
	createdRoleArn, err := iamClient.CreateFISRole(ctx, roleName, namespace, templateName)
	if err != nil {
		return "", fmt.Errorf("failed to create IAM role: %w", err)
	}

	return createdRoleArn, nil
}

// DeleteIAMRole deletes the IAM role for an experiment template
func DeleteIAMRole(ctx context.Context, iamClient *IAMClient, namespace, templateName string) error {
	roleName := GenerateRoleName(namespace, templateName)

	// Check if role exists
	exists, err := iamClient.RoleExists(ctx, roleName)
	if err != nil {
		return fmt.Errorf("failed to check if role exists: %w", err)
	}

	if !exists {
		// Role doesn't exist, nothing to delete
		return nil
	}

	// Delete the role
	err = iamClient.DeleteFISRole(ctx, roleName)
	if err != nil {
		return fmt.Errorf("failed to delete IAM role: %w", err)
	}

	return nil
}
