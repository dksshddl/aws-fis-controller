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

// ============================================================================
// Internal data structures for common conversion logic
// ============================================================================

type targetData struct {
	selectionMode string
	params        map[string]string
	filters       []types.ExperimentTemplateTargetInputFilter
}

type actionData struct {
	actionID    string
	description string
	params      map[string]string
	targets     map[string]string
	startAfter  []string
}

// ============================================================================
// Common conversion logic (shared between Create and Update)
// ============================================================================

func (c *FISClient) buildTargetData(target fisv1alpha1.TargetSpec, clusterIdentifier string) targetData {
	params := map[string]string{
		"clusterIdentifier": clusterIdentifier,
		"namespace":         defaultString(target.Namespace, "default"),
		"selectorType":      "labelSelector",
		"selectorValue":     buildLabelSelector(target.LabelSelector),
	}

	if target.Container != "" {
		params["targetContainerName"] = target.Container
	}

	var filters []types.ExperimentTemplateTargetInputFilter
	for _, f := range target.Filters {
		filters = append(filters, types.ExperimentTemplateTargetInputFilter{
			Path:   aws.String(f.Path),
			Values: f.Values,
		})
	}

	return targetData{
		selectionMode: parseScope(target.Scope),
		params:        params,
		filters:       filters,
	}
}

func (c *FISClient) buildActionData(action fisv1alpha1.ActionSpec, serviceAccount string) actionData {
	params := map[string]string{
		"duration": c.convertDuration(action.Duration),
	}

	if serviceAccount != "" {
		params["kubernetesServiceAccount"] = serviceAccount
	}

	for k, v := range action.Parameters {
		params[k] = v
	}

	return actionData{
		actionID:    c.convertActionType(action.Type),
		description: action.Description,
		params:      params,
		targets:     map[string]string{"Pods": action.Target},
		startAfter:  action.StartAfter,
	}
}

// ============================================================================
// Create API converters
// ============================================================================

func (c *FISClient) convertTargets(crdTargets []fisv1alpha1.TargetSpec, clusterIdentifier string) (map[string]types.CreateExperimentTemplateTargetInput, error) {
	targets := make(map[string]types.CreateExperimentTemplateTargetInput)
	for _, t := range crdTargets {
		data := c.buildTargetData(t, clusterIdentifier)
		targets[t.Name] = types.CreateExperimentTemplateTargetInput{
			ResourceType:  aws.String("aws:eks:pod"),
			SelectionMode: aws.String(data.selectionMode),
			Parameters:    data.params,
			Filters:       data.filters,
		}
	}
	return targets, nil
}

func (c *FISClient) convertActions(crdActions []fisv1alpha1.ActionSpec, serviceAccount string) (map[string]types.CreateExperimentTemplateActionInput, error) {
	actions := make(map[string]types.CreateExperimentTemplateActionInput)
	for _, a := range crdActions {
		data := c.buildActionData(a, serviceAccount)
		actions[a.Name] = types.CreateExperimentTemplateActionInput{
			ActionId:    aws.String(data.actionID),
			Description: aws.String(data.description),
			Parameters:  data.params,
			Targets:     data.targets,
			StartAfter:  data.startAfter,
		}
	}
	return actions, nil
}

func (c *FISClient) convertStopConditions(crdConditions []fisv1alpha1.StopCondition) []types.CreateExperimentTemplateStopConditionInput {
	var conditions []types.CreateExperimentTemplateStopConditionInput
	for _, cond := range crdConditions {
		input := types.CreateExperimentTemplateStopConditionInput{
			Source: aws.String(c.convertStopConditionSource(cond.Source)),
		}
		if cond.Value != "" {
			input.Value = aws.String(cond.Value)
		}
		conditions = append(conditions, input)
	}
	return conditions
}

func (c *FISClient) convertExperimentOptions(opts *fisv1alpha1.ExperimentOptions) *types.CreateExperimentTemplateExperimentOptionsInput {
	return &types.CreateExperimentTemplateExperimentOptionsInput{
		AccountTargeting:          types.AccountTargeting(opts.AccountTargeting),
		EmptyTargetResolutionMode: types.EmptyTargetResolutionMode(opts.EmptyTargetResolutionMode),
	}
}

func (c *FISClient) convertLogConfiguration(cfg *fisv1alpha1.LogConfiguration) *types.CreateExperimentTemplateLogConfigurationInput {
	input := &types.CreateExperimentTemplateLogConfigurationInput{
		LogSchemaVersion: aws.Int32(int32(cfg.LogSchemaVersion)),
	}
	if cfg.CloudWatchLogsConfiguration != nil {
		input.CloudWatchLogsConfiguration = &types.ExperimentTemplateCloudWatchLogsLogConfigurationInput{
			LogGroupArn: aws.String(cfg.CloudWatchLogsConfiguration.LogGroupArn),
		}
	}
	if cfg.S3Configuration != nil {
		input.S3Configuration = &types.ExperimentTemplateS3LogConfigurationInput{
			BucketName: aws.String(cfg.S3Configuration.BucketName),
			Prefix:     aws.String(cfg.S3Configuration.Prefix),
		}
	}
	return input
}

func (c *FISClient) convertExperimentReportConfiguration(cfg *fisv1alpha1.ExperimentReportConfiguration) *types.CreateExperimentTemplateReportConfigurationInput {
	input := &types.CreateExperimentTemplateReportConfigurationInput{}
	if cfg.PreExperimentDuration != "" {
		input.PreExperimentDuration = aws.String(c.convertDuration(cfg.PreExperimentDuration))
	}
	if cfg.PostExperimentDuration != "" {
		input.PostExperimentDuration = aws.String(c.convertDuration(cfg.PostExperimentDuration))
	}
	return input
}

func (c *FISClient) convertTags(crdTags []fisv1alpha1.Tag) map[string]string {
	tags := make(map[string]string)
	for _, tag := range crdTags {
		tags[tag.Key] = tag.Value
	}
	return tags
}

// ============================================================================
// Update API converters
// ============================================================================

func (c *FISClient) convertTargetsForUpdate(crdTargets []fisv1alpha1.TargetSpec, clusterIdentifier string) (map[string]types.UpdateExperimentTemplateTargetInput, error) {
	targets := make(map[string]types.UpdateExperimentTemplateTargetInput)
	for _, t := range crdTargets {
		data := c.buildTargetData(t, clusterIdentifier)
		targets[t.Name] = types.UpdateExperimentTemplateTargetInput{
			ResourceType:  aws.String("aws:eks:pod"),
			SelectionMode: aws.String(data.selectionMode),
			Parameters:    data.params,
			Filters:       data.filters,
		}
	}
	return targets, nil
}

func (c *FISClient) convertActionsForUpdate(crdActions []fisv1alpha1.ActionSpec, serviceAccount string) (map[string]types.UpdateExperimentTemplateActionInputItem, error) {
	actions := make(map[string]types.UpdateExperimentTemplateActionInputItem)
	for _, a := range crdActions {
		data := c.buildActionData(a, serviceAccount)
		actions[a.Name] = types.UpdateExperimentTemplateActionInputItem{
			ActionId:    aws.String(data.actionID),
			Description: aws.String(data.description),
			Parameters:  data.params,
			Targets:     data.targets,
			StartAfter:  data.startAfter,
		}
	}
	return actions, nil
}

func (c *FISClient) convertStopConditionsForUpdate(crdConditions []fisv1alpha1.StopCondition) []types.UpdateExperimentTemplateStopConditionInput {
	var conditions []types.UpdateExperimentTemplateStopConditionInput
	for _, cond := range crdConditions {
		input := types.UpdateExperimentTemplateStopConditionInput{
			Source: aws.String(c.convertStopConditionSource(cond.Source)),
		}
		if cond.Value != "" {
			input.Value = aws.String(cond.Value)
		}
		conditions = append(conditions, input)
	}
	return conditions
}

func (c *FISClient) convertExperimentOptionsForUpdate(opts *fisv1alpha1.ExperimentOptions) *types.UpdateExperimentTemplateExperimentOptionsInput {
	return &types.UpdateExperimentTemplateExperimentOptionsInput{
		EmptyTargetResolutionMode: types.EmptyTargetResolutionMode(opts.EmptyTargetResolutionMode),
	}
}

func (c *FISClient) convertLogConfigurationForUpdate(cfg *fisv1alpha1.LogConfiguration) *types.UpdateExperimentTemplateLogConfigurationInput {
	input := &types.UpdateExperimentTemplateLogConfigurationInput{
		LogSchemaVersion: aws.Int32(int32(cfg.LogSchemaVersion)),
	}
	if cfg.CloudWatchLogsConfiguration != nil {
		input.CloudWatchLogsConfiguration = &types.ExperimentTemplateCloudWatchLogsLogConfigurationInput{
			LogGroupArn: aws.String(cfg.CloudWatchLogsConfiguration.LogGroupArn),
		}
	}
	if cfg.S3Configuration != nil {
		input.S3Configuration = &types.ExperimentTemplateS3LogConfigurationInput{
			BucketName: aws.String(cfg.S3Configuration.BucketName),
			Prefix:     aws.String(cfg.S3Configuration.Prefix),
		}
	}
	return input
}

// ============================================================================
// Helper functions
// ============================================================================

// parseScope converts user-friendly scope format to AWS FIS selectionMode
// Examples: "ALL" -> "ALL", "3" -> "COUNT(3)", "50%" -> "PERCENT(50)"
func parseScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" || strings.EqualFold(scope, "ALL") {
		return "ALL"
	}
	if strings.HasSuffix(scope, "%") {
		return fmt.Sprintf("PERCENT(%s)", strings.TrimSuffix(scope, "%"))
	}
	return fmt.Sprintf("COUNT(%s)", scope)
}

func buildLabelSelector(labels map[string]string) string {
	var pairs []string
	for k, v := range labels {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(pairs, ",")
}

func defaultString(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
