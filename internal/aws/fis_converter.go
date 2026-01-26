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
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/fis/types"

	fisv1alpha1 "fis.dksshddl.dev/fis-controller/api/v1alpha1"
)

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
