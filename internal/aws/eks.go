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
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
)

// EKSClient wraps AWS EKS client
type EKSClient struct {
	client *eks.Client
}

// NewEKSClient creates a new EKS client using the provided AWS config
func NewEKSClient(awsConfig aws.Config) *EKSClient {
	return &EKSClient{
		client: eks.NewFromConfig(awsConfig),
	}
}

// GetClusterARN returns the EKS cluster ARN using EKS DescribeCluster API
func (c *EKSClient) GetClusterARN(ctx context.Context, clusterName string) (string, error) {
	output, err := c.client.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe cluster: %w", err)
	}

	return aws.ToString(output.Cluster.Arn), nil
}

// GetClusterARNFromConfig is a helper function to get cluster ARN using AWS config
func GetClusterARNFromConfig(ctx context.Context, awsConfig aws.Config, clusterName string) (string, error) {
	eksClient := NewEKSClient(awsConfig)
	return eksClient.GetClusterARN(ctx, clusterName)
}

// CreateAccessEntry creates an EKS access entry for the given IAM role
func (c *EKSClient) CreateAccessEntry(ctx context.Context, clusterName, principalArn string) error {
	input := &eks.CreateAccessEntryInput{
		ClusterName:  aws.String(clusterName),
		PrincipalArn: aws.String(principalArn),
		Username:     aws.String("fis-experiment"),
		Tags: map[string]string{
			"ManagedBy":              "aws-fis-controller",
			"kubernetes.io/role-arn": principalArn,
		},
	}

	_, err := c.client.CreateAccessEntry(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create access entry: %w", err)
	}

	return nil
}

// DeleteAccessEntry deletes an EKS access entry for the given IAM role
func (c *EKSClient) DeleteAccessEntry(ctx context.Context, clusterName, principalArn string) error {
	input := &eks.DeleteAccessEntryInput{
		ClusterName:  aws.String(clusterName),
		PrincipalArn: aws.String(principalArn),
	}

	_, err := c.client.DeleteAccessEntry(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete access entry: %w", err)
	}

	return nil
}

// AccessEntryExists checks if an access entry exists for the given IAM role
func (c *EKSClient) AccessEntryExists(ctx context.Context, clusterName, principalArn string) (bool, error) {
	input := &eks.DescribeAccessEntryInput{
		ClusterName:  aws.String(clusterName),
		PrincipalArn: aws.String(principalArn),
	}

	_, err := c.client.DescribeAccessEntry(ctx, input)
	if err != nil {
		// Check if it's a ResourceNotFoundException
		if isResourceNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to describe access entry: %w", err)
	}

	return true, nil
}

// EnsureAccessEntry ensures an access entry exists for the given IAM role
// If it doesn't exist, it creates one
func EnsureAccessEntry(ctx context.Context, eksClient *EKSClient, clusterName, principalArn string) error {
	exists, err := eksClient.AccessEntryExists(ctx, clusterName, principalArn)
	if err != nil {
		return fmt.Errorf("failed to check if access entry exists: %w", err)
	}

	if exists {
		// Access entry already exists
		return nil
	}

	// Create access entry
	if err := eksClient.CreateAccessEntry(ctx, clusterName, principalArn); err != nil {
		return fmt.Errorf("failed to create access entry: %w", err)
	}

	return nil
}

// DeleteAccessEntryIfExists deletes an access entry if it exists
func DeleteAccessEntryIfExists(ctx context.Context, eksClient *EKSClient, clusterName, principalArn string) error {
	exists, err := eksClient.AccessEntryExists(ctx, clusterName, principalArn)
	if err != nil {
		return fmt.Errorf("failed to check if access entry exists: %w", err)
	}

	if !exists {
		// Access entry doesn't exist, nothing to delete
		return nil
	}

	// Delete access entry
	if err := eksClient.DeleteAccessEntry(ctx, clusterName, principalArn); err != nil {
		return fmt.Errorf("failed to delete access entry: %w", err)
	}

	return nil
}

// isResourceNotFoundError checks if the error is a ResourceNotFoundException
func isResourceNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Check error message for ResourceNotFoundException
	return contains(err.Error(), "ResourceNotFoundException") ||
		contains(err.Error(), "No access entry found") ||
		contains(err.Error(), "not found")
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
