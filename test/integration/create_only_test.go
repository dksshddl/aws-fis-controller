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
	"testing"

	awsfis "fis.dksshddl.dev/fis-controller/internal/aws"
)

// TestCreateOnly creates an AWS FIS ExperimentTemplate without deleting it
// This allows you to inspect the created template in AWS Console
func TestCreateOnly(t *testing.T) {
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

	// Load the disk-stress-experiment template
	template, err := loadExperimentTemplate("../../config/samples/disk-stress-experiment.yaml")
	if err != nil {
		t.Fatalf("Failed to load experiment template: %v", err)
	}

	t.Logf("Creating AWS FIS ExperimentTemplate from: %s", template.Name)
	t.Logf("  Description: %s", template.Spec.Description)
	t.Logf("  Targets: %d", len(template.Spec.Targets))
	t.Logf("  Actions: %d", len(template.Spec.Actions))
	t.Logf("  Role ARN: %s", roleArn)
	t.Logf("  Cluster: %s", clusterIdentifier)
	t.Logf("  Service Account: %s", serviceAccount)

	// Create AWS FIS ExperimentTemplate
	templateID, err := fisClient.CreateExperimentTemplate(ctx, template, roleArn, clusterIdentifier, serviceAccount)
	if err != nil {
		t.Fatalf("Failed to create AWS FIS ExperimentTemplate: %v", err)
	}

	t.Logf("\n✓ Successfully created AWS FIS ExperimentTemplate!")
	t.Logf("  Template ID: %s", templateID)
	t.Logf("\nView in AWS Console:")
	t.Logf("  https://ap-northeast-2.console.aws.amazon.com/fis/home?region=ap-northeast-2#ExperimentTemplates/%s", templateID)
	t.Logf("\nTo delete this template, run:")
	t.Logf("  aws fis delete-experiment-template --id %s --region ap-northeast-2 --profile default", templateID)

	// Get the created template to show details
	createdTemplate, err := fisClient.GetExperimentTemplate(ctx, templateID)
	if err != nil {
		t.Errorf("Failed to get created template: %v", err)
		return
	}

	t.Logf("\nCreated Template Details:")
	t.Logf("  ID: %s", *createdTemplate.Id)
	t.Logf("  Description: %s", *createdTemplate.Description)
	t.Logf("  Role ARN: %s", *createdTemplate.RoleArn)
	t.Logf("  Targets: %d", len(createdTemplate.Targets))
	t.Logf("  Actions: %d", len(createdTemplate.Actions))

	// Show tags
	if len(createdTemplate.Tags) > 0 {
		t.Logf("\n  Tags:")
		for key, value := range createdTemplate.Tags {
			t.Logf("    %s: %s", key, value)
		}
	}

	// Show target details
	for targetName, target := range createdTemplate.Targets {
		t.Logf("\n  Target '%s':", targetName)
		t.Logf("    Resource Type: %s", *target.ResourceType)
		t.Logf("    Selection Mode: %s", *target.SelectionMode)
		if target.Parameters != nil {
			t.Logf("    Parameters:")
			for key, value := range target.Parameters {
				t.Logf("      %s: %s", key, value)
			}
		}
	}

	// Show action details
	for actionName, action := range createdTemplate.Actions {
		t.Logf("\n  Action '%s':", actionName)
		t.Logf("    Action ID: %s", *action.ActionId)
		if action.Description != nil {
			t.Logf("    Description: %s", *action.Description)
		}
		if action.Parameters != nil {
			t.Logf("    Parameters:")
			for key, value := range action.Parameters {
				t.Logf("      %s: %s", key, value)
			}
		}
		if action.Targets != nil {
			t.Logf("    Targets:")
			for key, value := range action.Targets {
				t.Logf("      %s: %s", key, value)
			}
		}
		if action.StartAfter != nil && len(action.StartAfter) > 0 {
			t.Logf("    Start After: %v", action.StartAfter)
		}
	}

	t.Logf("\n⚠️  NOTE: Template was created but NOT deleted. Please delete manually when done.")
}

// TestCreateOnlyJSON creates a template and outputs JSON for inspection
func TestCreateOnlyJSON(t *testing.T) {
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
		serviceAccount = "fis-pod-sa"
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

	// Load template
	template, err := loadExperimentTemplate("../../config/samples/disk-stress-experiment.yaml")
	if err != nil {
		t.Fatalf("Failed to load experiment template: %v", err)
	}

	// Create AWS FIS ExperimentTemplate
	templateID, err := fisClient.CreateExperimentTemplate(ctx, template, roleArn, clusterIdentifier, serviceAccount)
	if err != nil {
		t.Fatalf("Failed to create AWS FIS ExperimentTemplate: %v", err)
	}

	fmt.Printf("\n=== AWS FIS ExperimentTemplate Created ===\n")
	fmt.Printf("Template ID: %s\n", templateID)
	fmt.Printf("\nTo view as JSON, run:\n")
	fmt.Printf("aws fis get-experiment-template --id %s --region ap-northeast-2 --profile default\n", templateID)
	fmt.Printf("\nTo delete:\n")
	fmt.Printf("aws fis delete-experiment-template --id %s --region ap-northeast-2 --profile default\n", templateID)
}
