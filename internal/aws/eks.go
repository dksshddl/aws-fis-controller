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
