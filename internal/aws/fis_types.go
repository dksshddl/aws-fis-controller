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
	"strings"
)

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

// convertStopConditionSource converts CRD stop condition source to AWS format
func (c *FISClient) convertStopConditionSource(source string) string {
	if source == "cloudwatch-alarm" {
		return "aws:cloudwatch:alarm"
	}
	return source
}
