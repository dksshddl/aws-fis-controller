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

package experimenttemplate

import (
	"context"
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fisv1alpha1 "fis.dksshddl.dev/fis-controller/api/v1alpha1"
)

func TestReconciler(t *testing.T) {
	// Create a fake client
	scheme := runtime.NewScheme()
	_ = fisv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Create reconciler
	reconciler := &Reconciler{
		Client:     fakeClient,
		Scheme:     scheme,
		ClusterARN: "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster",
	}

	if reconciler == nil {
		t.Fatal("Failed to create reconciler")
	}

	// Basic test to ensure reconciler is created properly
	if reconciler.Client == nil {
		t.Error("Client should not be nil")
	}

	if reconciler.Scheme == nil {
		t.Error("Scheme should not be nil")
	}
}

func TestGetRequiredParametersWithEnvVars(t *testing.T) {
	// Save original env vars
	origRoleArn := os.Getenv("FIS_ROLE_ARN")
	origCluster := os.Getenv("CLUSTER_IDENTIFIER")

	// Set test env vars
	os.Setenv("FIS_ROLE_ARN", "arn:aws:iam::123456789012:role/env-role")
	os.Setenv("CLUSTER_IDENTIFIER", "arn:aws:eks:ap-northeast-2:123456789012:cluster/env-cluster")

	// Restore original env vars after test
	defer func() {
		if origRoleArn != "" {
			os.Setenv("FIS_ROLE_ARN", origRoleArn)
		} else {
			os.Unsetenv("FIS_ROLE_ARN")
		}
		if origCluster != "" {
			os.Setenv("CLUSTER_IDENTIFIER", origCluster)
		} else {
			os.Unsetenv("CLUSTER_IDENTIFIER")
		}
	}()

	scheme := runtime.NewScheme()
	_ = fisv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	reconciler := &Reconciler{
		Client:     fakeClient,
		Scheme:     scheme,
		ClusterARN: "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster",
	}

	// Test with environment variables (should take precedence)
	template := &fisv1alpha1.ExperimentTemplate{}

	ctx := context.Background()
	roleArn, clusterIdentifier, err := reconciler.getRequiredParameters(ctx, template)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
		return
	}

	if roleArn != "arn:aws:iam::123456789012:role/env-role" {
		t.Errorf("Expected roleArn from env 'arn:aws:iam::123456789012:role/env-role', got: %s", roleArn)
	}

	if clusterIdentifier != "arn:aws:eks:ap-northeast-2:123456789012:cluster/env-cluster" {
		t.Errorf("Expected clusterIdentifier from env 'arn:aws:eks:ap-northeast-2:123456789012:cluster/env-cluster', got: %s", clusterIdentifier)
	}
}

func TestGetRequiredParametersWithAnnotations(t *testing.T) {
	// Clear env vars for this test
	origRoleArn := os.Getenv("FIS_ROLE_ARN")
	origCluster := os.Getenv("CLUSTER_IDENTIFIER")

	os.Unsetenv("FIS_ROLE_ARN")
	os.Unsetenv("CLUSTER_IDENTIFIER")

	// Restore original env vars after test
	defer func() {
		if origRoleArn != "" {
			os.Setenv("FIS_ROLE_ARN", origRoleArn)
		}
		if origCluster != "" {
			os.Setenv("CLUSTER_IDENTIFIER", origCluster)
		}
	}()

	scheme := runtime.NewScheme()
	_ = fisv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	reconciler := &Reconciler{
		Client:     fakeClient,
		Scheme:     scheme,
		ClusterARN: "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster",
	}

	// Test with role in status and cluster in annotation
	template := &fisv1alpha1.ExperimentTemplate{}
	template.Annotations = map[string]string{
		"fis.dksshddl.dev/cluster-identifier": "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster",
	}
	template.Status.RoleArn = "arn:aws:iam::123456789012:role/status-role"

	ctx := context.Background()
	roleArn, clusterIdentifier, err := reconciler.getRequiredParameters(ctx, template)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
		return
	}

	if roleArn != "arn:aws:iam::123456789012:role/status-role" {
		t.Errorf("Expected roleArn from status 'arn:aws:iam::123456789012:role/status-role', got: %s", roleArn)
	}

	if clusterIdentifier != "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster" {
		t.Errorf("Expected clusterIdentifier from annotation 'arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster', got: %s", clusterIdentifier)
	}
}

func TestGetRequiredParametersWithDefaults(t *testing.T) {
	// Clear env vars for this test
	origRoleArn := os.Getenv("FIS_ROLE_ARN")
	origCluster := os.Getenv("CLUSTER_IDENTIFIER")

	os.Unsetenv("FIS_ROLE_ARN")
	os.Unsetenv("CLUSTER_IDENTIFIER")

	// Restore original env vars after test
	defer func() {
		if origRoleArn != "" {
			os.Setenv("FIS_ROLE_ARN", origRoleArn)
		}
		if origCluster != "" {
			os.Setenv("CLUSTER_IDENTIFIER", origCluster)
		}
	}()

	scheme := runtime.NewScheme()
	_ = fisv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	reconciler := &Reconciler{
		Client:     fakeClient,
		Scheme:     scheme,
		ClusterARN: "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster",
	}

	// Test with minimal config
	template := &fisv1alpha1.ExperimentTemplate{}
	template.Annotations = map[string]string{
		"fis.dksshddl.dev/cluster-identifier": "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster",
	}
	template.Status.RoleArn = "arn:aws:iam::123456789012:role/test-role"

	ctx := context.Background()
	roleArn, clusterIdentifier, err := reconciler.getRequiredParameters(ctx, template)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
		return
	}

	if roleArn != "arn:aws:iam::123456789012:role/test-role" {
		t.Errorf("Expected roleArn 'arn:aws:iam::123456789012:role/test-role', got: %s", roleArn)
	}

	if clusterIdentifier != "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster" {
		t.Errorf("Expected clusterIdentifier 'arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster', got: %s", clusterIdentifier)
	}
}
