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
	"strings"

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
	client *fis.Client
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
		config.WithSharedConfigProfile("default"),
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
		client: fis.NewFromConfig(awsConfig),
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

	// Convert tags
	if len(template.Spec.Tags) > 0 {
		input.Tags = c.convertTags(template.Spec.Tags)
	}

	// Create the experiment template
	output, err := c.client.CreateExperimentTemplate(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to create experiment template: %w", err)
	}

	return aws.ToString(output.ExperimentTemplate.Id), nil
}

// convertTargets converts CRD targets to AWS FIS targets
func (c *FISClient) convertTargets(crdTargets []fisv1alpha1.TargetSpec, clusterIdentifier string) (map[string]types.CreateExperimentTemplateTargetInput, error) {
	targets := make(map[string]types.CreateExperimentTemplateTargetInput)

	for _, target := range crdTargets {
		fisTarget := types.CreateExperimentTemplateTargetInput{
			ResourceType:  aws.String("aws:eks:pod"),
			SelectionMode: aws.String(target.SelectionMode),
			Parameters:    make(map[string]string),
		}

		// Set cluster identifier
		fisTarget.Parameters["clusterIdentifier"] = clusterIdentifier

		// Set namespace
		namespace := target.Namespace
		if namespace == "" {
			namespace = "default"
		}
		fisTarget.Parameters["namespace"] = namespace

		// Set label selector
		fisTarget.Parameters["selectorType"] = "labelSelector"

		// Convert labelSelector map to string (e.g., "app=nginx,tier=frontend")
		var selectorPairs []string
		for key, value := range target.LabelSelector {
			selectorPairs = append(selectorPairs, fmt.Sprintf("%s=%s", key, value))
		}
		fisTarget.Parameters["selectorValue"] = strings.Join(selectorPairs, ",")

		// Set target container name if specified
		if target.TargetContainerName != "" {
			fisTarget.Parameters["targetContainerName"] = target.TargetContainerName
		}

		// Convert filters if present
		if len(target.Filters) > 0 {
			var filters []types.ExperimentTemplateTargetInputFilter
			for _, filter := range target.Filters {
				filters = append(filters, types.ExperimentTemplateTargetInputFilter{
					Path:   aws.String(filter.Path),
					Values: filter.Values,
				})
			}
			fisTarget.Filters = filters
		}

		targets[target.Name] = fisTarget
	}

	return targets, nil
}

// convertActions converts CRD actions to AWS FIS actions
func (c *FISClient) convertActions(crdActions []fisv1alpha1.ActionSpec, serviceAccount string) (map[string]types.CreateExperimentTemplateActionInput, error) {
	actions := make(map[string]types.CreateExperimentTemplateActionInput)

	for _, action := range crdActions {
		fisAction := types.CreateExperimentTemplateActionInput{
			ActionId:    aws.String(c.convertActionType(action.Type)),
			Description: aws.String(action.Description),
			Parameters:  make(map[string]string),
			Targets:     make(map[string]string),
		}

		// Convert duration from Kubernetes format (5m) to AWS format (PT5M)
		fisAction.Parameters["duration"] = c.convertDuration(action.Duration)

		// Set kubernetes service account
		fisAction.Parameters["kubernetesServiceAccount"] = serviceAccount

		// Copy other parameters
		for key, value := range action.Parameters {
			fisAction.Parameters[key] = value
		}

		// Set target reference
		// AWS FIS uses "Pods" as the target key for EKS pod actions
		fisAction.Targets["Pods"] = action.Target

		// Set start after dependencies
		if len(action.StartAfter) > 0 {
			fisAction.StartAfter = action.StartAfter
		}

		actions[action.Name] = fisAction
	}

	return actions, nil
}

// convertActionType converts CRD action type to AWS FIS action ID
func (c *FISClient) convertActionType(actionType string) string {
	actionMap := map[string]string{
		"pod-cpu-stress":          "aws:eks:pod-cpu-stress",
		"pod-memory-stress":       "aws:eks:pod-memory-stress",
		"pod-io-stress":           "aws:eks:pod-io-stress",
		"pod-network-latency":     "aws:eks:pod-network-latency",
		"pod-network-packet-loss": "aws:eks:pod-network-packet-loss",
		"pod-delete":              "aws:eks:pod-delete",
	}

	if awsActionId, ok := actionMap[actionType]; ok {
		return awsActionId
	}

	// If not found, assume it's already in AWS format
	return actionType
}

// convertDuration converts Kubernetes duration format to AWS ISO 8601 duration
// e.g., "5m" -> "PT5M", "1h" -> "PT1H", "30s" -> "PT30S"
func (c *FISClient) convertDuration(duration string) string {
	if strings.HasPrefix(duration, "PT") {
		return duration // Already in AWS format
	}

	// Convert from Kubernetes format
	duration = strings.ToUpper(duration)
	return "PT" + duration
}

// convertStopConditions converts CRD stop conditions to AWS FIS stop conditions
func (c *FISClient) convertStopConditions(crdConditions []fisv1alpha1.StopCondition) []types.CreateExperimentTemplateStopConditionInput {
	var conditions []types.CreateExperimentTemplateStopConditionInput

	for _, condition := range crdConditions {
		fisCondition := types.CreateExperimentTemplateStopConditionInput{
			Source: aws.String(c.convertStopConditionSource(condition.Source)),
		}

		if condition.Value != "" {
			fisCondition.Value = aws.String(condition.Value)
		}

		conditions = append(conditions, fisCondition)
	}

	return conditions
}

// convertStopConditionSource converts CRD stop condition source to AWS format
func (c *FISClient) convertStopConditionSource(source string) string {
	if source == "cloudwatch-alarm" {
		return "aws:cloudwatch:alarm"
	}
	return source
}

// convertExperimentOptions converts CRD experiment options to AWS FIS format
func (c *FISClient) convertExperimentOptions(options *fisv1alpha1.ExperimentOptions) *types.CreateExperimentTemplateExperimentOptionsInput {
	return &types.CreateExperimentTemplateExperimentOptionsInput{
		AccountTargeting:          types.AccountTargeting(options.AccountTargeting),
		EmptyTargetResolutionMode: types.EmptyTargetResolutionMode(options.EmptyTargetResolutionMode),
	}
}

// convertLogConfiguration converts CRD log configuration to AWS FIS format
func (c *FISClient) convertLogConfiguration(logConfig *fisv1alpha1.LogConfiguration) *types.CreateExperimentTemplateLogConfigurationInput {
	config := &types.CreateExperimentTemplateLogConfigurationInput{
		LogSchemaVersion: aws.Int32(int32(logConfig.LogSchemaVersion)),
	}

	if logConfig.CloudWatchLogsConfiguration != nil {
		config.CloudWatchLogsConfiguration = &types.ExperimentTemplateCloudWatchLogsLogConfigurationInput{
			LogGroupArn: aws.String(logConfig.CloudWatchLogsConfiguration.LogGroupArn),
		}
	}

	if logConfig.S3Configuration != nil {
		config.S3Configuration = &types.ExperimentTemplateS3LogConfigurationInput{
			BucketName: aws.String(logConfig.S3Configuration.BucketName),
		}
		if logConfig.S3Configuration.Prefix != "" {
			config.S3Configuration.Prefix = aws.String(logConfig.S3Configuration.Prefix)
		}
	}

	return config
}

// convertExperimentReportConfiguration converts CRD report configuration to AWS FIS format
func (c *FISClient) convertExperimentReportConfiguration(reportConfig *fisv1alpha1.ExperimentReportConfiguration) *types.CreateExperimentTemplateReportConfigurationInput {
	config := &types.CreateExperimentTemplateReportConfigurationInput{}

	if reportConfig.PreExperimentDuration != "" {
		config.PreExperimentDuration = aws.String(c.convertDuration(reportConfig.PreExperimentDuration))
	}

	if reportConfig.PostExperimentDuration != "" {
		config.PostExperimentDuration = aws.String(c.convertDuration(reportConfig.PostExperimentDuration))
	}

	// TODO: Implement DataSources and Outputs conversion
	// The exact type names need to be verified from AWS SDK documentation
	// if reportConfig.DataSources != nil && len(reportConfig.DataSources.CloudWatchDashboards) > 0 {
	// 	...
	// }

	// if reportConfig.Outputs != nil && reportConfig.Outputs.S3Configuration != nil {
	// 	...
	// }

	return config
}

// convertTags converts CRD tags to AWS FIS tags
func (c *FISClient) convertTags(crdTags []fisv1alpha1.Tag) map[string]string {
	tags := make(map[string]string)
	for _, tag := range crdTags {
		tags[tag.Key] = tag.Value
	}
	return tags
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
