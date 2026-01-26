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

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"

	fisv1alpha1 "fis.dksshddl.dev/fis-controller/api/v1alpha1"
	awsfis "fis.dksshddl.dev/fis-controller/internal/aws"
)

// TestFISAPIIntegration tests the full flow of creating, updating, and deleting FIS experiment templates
func TestFISAPIIntegration(t *testing.T) {
	// Skip if not running integration tests
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=true to run")
	}

	// Required environment variables
	roleArn := os.Getenv("FIS_ROLE_ARN")
	if roleArn == "" {
		t.Fatal("FIS_ROLE_ARN environment variable is required")
	}

	clusterIdentifier := os.Getenv("CLUSTER_IDENTIFIER")
	if clusterIdentifier == "" {
		t.Fatal("CLUSTER_IDENTIFIER environment variable is required")
	}

	serviceAccount := os.Getenv("SERVICE_ACCOUNT")
	if serviceAccount == "" {
		serviceAccount = "fis-pod-sa" // default
	}

	ctx := context.Background()

	// Create FIS client
	fisClient, err := awsfis.NewFISClient(ctx, awsfis.FISConfig{
		Region:     "ap-northeast-2",
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("Failed to create FIS client: %v", err)
	}

	// Test cases
	testCases := []struct {
		name         string
		sampleFile   string
		shouldCreate bool
		shouldUpdate bool
	}{
		{
			name:         "Disk Stress Experiment",
			sampleFile:   "../../config/samples/disk-stress-experiment.yaml",
			shouldCreate: true,
			shouldUpdate: true,
		},
		{
			name:         "Memory Stress Experiment",
			sampleFile:   "../../config/samples/memory-stress-experiment.yaml",
			shouldCreate: true,
			shouldUpdate: false,
		},
		{
			name:         "Network Latency Experiment",
			sampleFile:   "../../config/samples/network-latency-experiment.yaml",
			shouldCreate: true,
			shouldUpdate: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Load CRD from file
			template, err := loadExperimentTemplate(tc.sampleFile)
			if err != nil {
				t.Fatalf("Failed to load experiment template: %v", err)
			}

			t.Logf("Testing with template: %s", template.Name)

			// Test Create
			if tc.shouldCreate {
				templateID, err := testCreate(ctx, fisClient, template, roleArn, clusterIdentifier, serviceAccount)
				if err != nil {
					t.Fatalf("Create test failed: %v", err)
				}
				t.Logf("✓ Successfully created AWS FIS ExperimentTemplate: %s", templateID)

				// Test Get
				if err := testGet(ctx, fisClient, templateID); err != nil {
					t.Errorf("Get test failed: %v", err)
				} else {
					t.Logf("✓ Successfully retrieved AWS FIS ExperimentTemplate")
				}

				// Test Update
				if tc.shouldUpdate {
					if err := testUpdate(ctx, fisClient, template, templateID, roleArn, clusterIdentifier, serviceAccount); err != nil {
						t.Errorf("Update test failed: %v", err)
					} else {
						t.Logf("✓ Successfully updated AWS FIS ExperimentTemplate")
					}
				}

				// Test Delete
				if err := testDelete(ctx, fisClient, templateID); err != nil {
					t.Errorf("Delete test failed: %v", err)
				} else {
					t.Logf("✓ Successfully deleted AWS FIS ExperimentTemplate")
				}
			}
		})
	}
}

// loadExperimentTemplate loads an ExperimentTemplate from a YAML file
func loadExperimentTemplate(filePath string) (*fisv1alpha1.ExperimentTemplate, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", absPath, err)
	}

	template := &fisv1alpha1.ExperimentTemplate{}
	if err := yaml.Unmarshal(data, template); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	return template, nil
}

// testCreate tests creating an AWS FIS experiment template
func testCreate(ctx context.Context, client *awsfis.FISClient, template *fisv1alpha1.ExperimentTemplate, roleArn, clusterIdentifier, serviceAccount string) (string, error) {
	templateID, err := client.CreateExperimentTemplate(ctx, template, roleArn, clusterIdentifier, serviceAccount)
	if err != nil {
		return "", fmt.Errorf("CreateExperimentTemplate failed: %w", err)
	}

	if templateID == "" {
		return "", fmt.Errorf("CreateExperimentTemplate returned empty templateID")
	}

	return templateID, nil
}

// testGet tests retrieving an AWS FIS experiment template
func testGet(ctx context.Context, client *awsfis.FISClient, templateID string) error {
	template, err := client.GetExperimentTemplate(ctx, templateID)
	if err != nil {
		return fmt.Errorf("GetExperimentTemplate failed: %w", err)
	}

	if template == nil {
		return fmt.Errorf("GetExperimentTemplate returned nil template")
	}

	if template.Id == nil || *template.Id != templateID {
		return fmt.Errorf("GetExperimentTemplate returned wrong template ID")
	}

	return nil
}

// testUpdate tests updating an AWS FIS experiment template
func testUpdate(ctx context.Context, client *awsfis.FISClient, template *fisv1alpha1.ExperimentTemplate, templateID, roleArn, clusterIdentifier, serviceAccount string) error {
	// Modify the template description
	originalDesc := template.Spec.Description
	template.Spec.Description = originalDesc + " (Updated)"

	err := client.UpdateExperimentTemplate(ctx, template, templateID, roleArn, clusterIdentifier, serviceAccount)
	if err != nil {
		return fmt.Errorf("UpdateExperimentTemplate failed: %w", err)
	}

	// Verify the update
	updatedTemplate, err := client.GetExperimentTemplate(ctx, templateID)
	if err != nil {
		return fmt.Errorf("Failed to get updated template: %w", err)
	}

	if updatedTemplate.Description == nil || *updatedTemplate.Description != template.Spec.Description {
		return fmt.Errorf("Update verification failed: description not updated")
	}

	return nil
}

// testDelete tests deleting an AWS FIS experiment template
func testDelete(ctx context.Context, client *awsfis.FISClient, templateID string) error {
	err := client.DeleteExperimentTemplate(ctx, templateID)
	if err != nil {
		return fmt.Errorf("DeleteExperimentTemplate failed: %w", err)
	}

	return nil
}

// TestConversionLogic tests the conversion logic without making actual AWS API calls
func TestConversionLogic(t *testing.T) {
	testCases := []struct {
		name       string
		sampleFile string
	}{
		{
			name:       "Disk Stress Experiment",
			sampleFile: "../../config/samples/disk-stress-experiment.yaml",
		},
		{
			name:       "Full Featured Experiment",
			sampleFile: "../../config/samples/full-featured-experiment.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			template, err := loadExperimentTemplate(tc.sampleFile)
			if err != nil {
				t.Fatalf("Failed to load experiment template: %v", err)
			}

			// Validate basic structure
			if template.Spec.Description == "" {
				t.Error("Description is empty")
			}

			if len(template.Spec.Targets) == 0 {
				t.Error("No targets defined")
			}

			if len(template.Spec.Actions) == 0 {
				t.Error("No actions defined")
			}

			// Validate targets
			for i, target := range template.Spec.Targets {
				if target.Name == "" {
					t.Errorf("Target %d has empty name", i)
				}
				if len(target.LabelSelector) == 0 {
					t.Errorf("Target %s has no label selectors", target.Name)
				}
			}

			// Validate actions
			for i, action := range template.Spec.Actions {
				if action.Name == "" {
					t.Errorf("Action %d has empty name", i)
				}
				if action.Type == "" {
					t.Errorf("Action %s has empty type", action.Name)
				}
				if action.Duration == "" {
					t.Errorf("Action %s has empty duration", action.Name)
				}
				if action.Target == "" {
					t.Errorf("Action %s has no target reference", action.Name)
				}
			}

			t.Logf("✓ Template %s passed validation", template.Name)
		})
	}
}
