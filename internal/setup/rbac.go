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

package setup

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	FISServiceAccountName = "fis-pod-sa"
	FISRoleName           = "fis-pod-role"
	FISRoleBindingName    = "fis-pod-rolebinding"
)

// SetupFISRBAC creates ServiceAccount, Role, and RoleBinding for FIS pods
func SetupFISRBAC(ctx context.Context, namespace string) error {
	setupLog := ctrl.Log.WithName("setup-rbac")

	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	// Create ServiceAccount
	if err := createServiceAccount(ctx, clientset, namespace); err != nil {
		setupLog.Error(err, "failed to create ServiceAccount")
		return err
	}
	setupLog.Info("ServiceAccount created or already exists", "name", FISServiceAccountName, "namespace", namespace)

	// Create Role
	if err := createRole(ctx, clientset, namespace); err != nil {
		setupLog.Error(err, "failed to create Role")
		return err
	}
	setupLog.Info("Role created or already exists", "name", FISRoleName, "namespace", namespace)

	// Create RoleBinding
	if err := createRoleBinding(ctx, clientset, namespace); err != nil {
		setupLog.Error(err, "failed to create RoleBinding")
		return err
	}
	setupLog.Info("RoleBinding created or already exists", "name", FISRoleBindingName, "namespace", namespace)

	return nil
}

func createServiceAccount(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      FISServiceAccountName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "fis-controller",
				"app.kubernetes.io/component":  "fis-pod",
				"app.kubernetes.io/managed-by": "fis-controller",
			},
		},
	}

	_, err := clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create ServiceAccount: %w", err)
	}

	return nil
}

func createRole(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      FISRoleName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "fis-controller",
				"app.kubernetes.io/component":  "fis-pod",
				"app.kubernetes.io/managed-by": "fis-controller",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/log"},
				Verbs:     []string{"get"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "replicasets", "statefulsets"},
				Verbs:     []string{"get", "list", "patch", "update"},
			},
		},
	}

	_, err := clientset.RbacV1().Roles(namespace).Create(ctx, role, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create Role: %w", err)
	}

	return nil
}

func createRoleBinding(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      FISRoleBindingName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "fis-controller",
				"app.kubernetes.io/component":  "fis-pod",
				"app.kubernetes.io/managed-by": "fis-controller",
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      FISServiceAccountName,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     FISRoleName,
		},
	}

	_, err := clientset.RbacV1().RoleBindings(namespace).Create(ctx, roleBinding, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create RoleBinding: %w", err)
	}

	return nil
}
