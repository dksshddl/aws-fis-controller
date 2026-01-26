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
		Client: fakeClient,
		Scheme: scheme,
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

func TestGetRequiredParameters(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fisv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	reconciler := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Test with annotations
	template := &fisv1alpha1.ExperimentTemplate{}
	template.Annotations = map[string]string{
		"fis.dksshddl.dev/role-arn":           "arn:aws:iam::123456789012:role/test-role",
		"fis.dksshddl.dev/cluster-identifier": "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster",
		"fis.dksshddl.dev/service-account":    "test-sa",
	}

	roleArn, clusterIdentifier, serviceAccount, err := reconciler.getRequiredParameters(template)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if roleArn != "arn:aws:iam::123456789012:role/test-role" {
		t.Errorf("Expected roleArn to be set, got: %s", roleArn)
	}

	if clusterIdentifier != "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster" {
		t.Errorf("Expected clusterIdentifier to be set, got: %s", clusterIdentifier)
	}

	if serviceAccount != "test-sa" {
		t.Errorf("Expected serviceAccount to be 'test-sa', got: %s", serviceAccount)
	}
}

func TestGetRequiredParametersWithDefaults(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = fisv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	reconciler := &Reconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Test with partial annotations (service account should default)
	template := &fisv1alpha1.ExperimentTemplate{}
	template.Annotations = map[string]string{
		"fis.dksshddl.dev/role-arn":           "arn:aws:iam::123456789012:role/test-role",
		"fis.dksshddl.dev/cluster-identifier": "arn:aws:eks:ap-northeast-2:123456789012:cluster/test-cluster",
	}

	_, _, serviceAccount, err := reconciler.getRequiredParameters(template)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if serviceAccount != "fis-pod-sa" {
		t.Errorf("Expected default serviceAccount 'fis-pod-sa', got: %s", serviceAccount)
	}
}
