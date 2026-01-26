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

package experiment

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
