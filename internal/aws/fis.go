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
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/fis"
	"github.com/aws/aws-sdk-go-v2/service/fis/types"
	"github.com/google/uuid"

	fisv1alpha1 "fis.dksshddl.dev/fis-controller/api/v1alpha1"
)

// FISClient wraps AWS FIS client
type FISClient struct {
	client    *fis.Client
	awsConfig aws.Config
}

// FISConfig holds configuration for FIS client
type FISConfig struct {
	Region     string
	MaxRetries int
}

// NewFISClient creates a new FIS client
func NewFISClient(ctx context.Context, cfg FISConfig) (*FISClient, error) {
	// Determine region
	region := cfg.Region
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
		if region == "" {
			region = os.Getenv("AWS_REGION")
		}
		if region == "" {
			// Try to get region from EC2 metadata
			imdsClient := imds.NewFromConfig(aws.Config{})
			output, err := imdsClient.GetRegion(ctx, &imds.GetRegionInput{})
			if err == nil && output.Region != "" {
				region = output.Region
			} else {
				// Default to ap-northeast-2
				region = "ap-northeast-2"
			}
		}
	}

	// Set default max retries
	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	// Load AWS config
	awsConfig, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.MaxAttempts = maxRetries
			})
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &FISClient{
		client:    fis.NewFromConfig(awsConfig),
		awsConfig: awsConfig,
	}, nil
}

// CreateExperimentTemplate creates an AWS FIS experiment template from CRD spec
func (c *FISClient) CreateExperimentTemplate(ctx context.Context, template *fisv1alpha1.ExperimentTemplate, roleArn, clusterIdentifier, serviceAccount string) (string, error) {
	input := &fis.CreateExperimentTemplateInput{
		ClientToken: aws.String(uuid.New().String()),
		Description: aws.String(template.Spec.Description),
		RoleArn:     aws.String(roleArn),
	}

	// Convert targets
	targets, err := c.convertTargets(template.Spec.Targets, clusterIdentifier)
	if err != nil {
		return "", fmt.Errorf("failed to convert targets: %w", err)
	}
	input.Targets = targets

	// Convert actions
	actions, err := c.convertActions(template.Spec.Actions, serviceAccount)
	if err != nil {
		return "", fmt.Errorf("failed to convert actions: %w", err)
	}
	input.Actions = actions

	// Convert stop conditions
	if len(template.Spec.StopConditions) > 0 {
		input.StopConditions = c.convertStopConditions(template.Spec.StopConditions)
	}

	// Convert experiment options
	if template.Spec.ExperimentOptions != nil {
		input.ExperimentOptions = c.convertExperimentOptions(template.Spec.ExperimentOptions)
	}

	// Convert log configuration
	if template.Spec.LogConfiguration != nil {
		input.LogConfiguration = c.convertLogConfiguration(template.Spec.LogConfiguration)
	}

	// Convert experiment report configuration
	if template.Spec.ExperimentReportConfiguration != nil {
		input.ExperimentReportConfiguration = c.convertExperimentReportConfiguration(template.Spec.ExperimentReportConfiguration)
	}

	// Convert tags and add management tags
	tags := make(map[string]string)
	if len(template.Spec.Tags) > 0 {
		tags = c.convertTags(template.Spec.Tags)
	}

	// Add management tags to identify controller-managed resources
	tags["ManagedBy"] = "aws-fis-controller"
	tags["kubernetes.io/name"] = template.Name
	tags["kubernetes.io/namespace"] = template.Namespace

	input.Tags = tags

	// Create the experiment template
	output, err := c.client.CreateExperimentTemplate(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to create experiment template: %w", err)
	}

	return aws.ToString(output.ExperimentTemplate.Id), nil
}

// UpdateExperimentTemplate updates an AWS FIS experiment template
// Note: AWS FIS Update API has limitations, only description is updated for now
func (c *FISClient) UpdateExperimentTemplate(ctx context.Context, template *fisv1alpha1.ExperimentTemplate, templateID, roleArn, clusterIdentifier, serviceAccount string) error {
	input := &fis.UpdateExperimentTemplateInput{
		Id:          aws.String(templateID),
		Description: aws.String(template.Spec.Description),
	}

	// Update the experiment template (only description for now)
	// For full updates, consider delete + recreate pattern
	_, err := c.client.UpdateExperimentTemplate(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update experiment template: %w", err)
	}

	return nil
}

// DeleteExperimentTemplate deletes an AWS FIS experiment template
func (c *FISClient) DeleteExperimentTemplate(ctx context.Context, templateID string) error {
	input := &fis.DeleteExperimentTemplateInput{
		Id: aws.String(templateID),
	}

	_, err := c.client.DeleteExperimentTemplate(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete experiment template: %w", err)
	}

	return nil
}

// GetExperimentTemplate gets an AWS FIS experiment template
func (c *FISClient) GetExperimentTemplate(ctx context.Context, templateID string) (*types.ExperimentTemplate, error) {
	input := &fis.GetExperimentTemplateInput{
		Id: aws.String(templateID),
	}

	output, err := c.client.GetExperimentTemplate(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get experiment template: %w", err)
	}

	return output.ExperimentTemplate, nil
}

// StartExperiment starts an AWS FIS experiment from a template
func (c *FISClient) StartExperiment(ctx context.Context, experiment *fisv1alpha1.Experiment) (string, error) {
	// Use the resolved template ID from status
	templateID := experiment.Status.TemplateID
	if templateID == "" {
		return "", fmt.Errorf("template ID not resolved in status")
	}

	input := &fis.StartExperimentInput{
		ExperimentTemplateId: aws.String(templateID),
	}

	// Set client token if provided, otherwise generate one
	if experiment.Spec.ClientToken != "" {
		input.ClientToken = aws.String(experiment.Spec.ClientToken)
	} else {
		input.ClientToken = aws.String(uuid.New().String())
	}

	// Convert tags
	if len(experiment.Spec.Tags) > 0 {
		tags := c.convertTags(experiment.Spec.Tags)
		// Add management tags
		tags["ManagedBy"] = "aws-fis-controller"
		tags["kubernetes.io/name"] = experiment.Name
		tags["kubernetes.io/namespace"] = experiment.Namespace
		input.Tags = tags
	} else {
		// Add management tags even if no user tags
		input.Tags = map[string]string{
			"ManagedBy":               "aws-fis-controller",
			"kubernetes.io/name":      experiment.Name,
			"kubernetes.io/namespace": experiment.Namespace,
		}
	}

	// Start the experiment
	output, err := c.client.StartExperiment(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to start experiment: %w", err)
	}

	return aws.ToString(output.Experiment.Id), nil
}

// GetExperiment gets the current state of an AWS FIS experiment
func (c *FISClient) GetExperiment(ctx context.Context, experimentID string) (*types.Experiment, error) {
	input := &fis.GetExperimentInput{
		Id: aws.String(experimentID),
	}

	output, err := c.client.GetExperiment(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get experiment: %w", err)
	}

	return output.Experiment, nil
}

// StopExperiment stops a running AWS FIS experiment
func (c *FISClient) StopExperiment(ctx context.Context, experimentID string) error {
	input := &fis.StopExperimentInput{
		Id: aws.String(experimentID),
	}

	_, err := c.client.StopExperiment(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to stop experiment: %w", err)
	}

	return nil
}

// GetAWSConfig returns the AWS config for creating other clients
func (c *FISClient) GetAWSConfig() aws.Config {
	return c.awsConfig
}
