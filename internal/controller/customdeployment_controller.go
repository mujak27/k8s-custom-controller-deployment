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

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workloadsv1alpha1 "example.com/custom-controller-deployment/api/v1alpha1"
)

// CustomDeploymentReconciler reconciles a CustomDeployment object
type CustomDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=workloads.example.com,resources=customdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=workloads.example.com,resources=customdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=workloads.example.com,resources=customdeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create

// Reconcile creates the workload; initial replicas are never reapplied to an existing Deployment.
func (r *CustomDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	workload := &workloadsv1alpha1.CustomDeployment{}
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !workload.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, deployment); err == nil {
		if !metav1.IsControlledBy(deployment, workload) {
			return ctrl.Result{}, fmt.Errorf("deployment %s is not controlled by CustomDeployment %s", req.NamespacedName, workload.Name)
		}
		return ctrl.Result{}, nil
	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	labels := map[string]string{"workloads.example.com/customdeployment-uid": string(workload.UID)}
	deployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: workload.Name, Namespace: workload.Namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: new(workload.Spec.Replicas),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name: "workload", Image: workload.Spec.Image, Resources: workload.Spec.Resources,
				}}},
			},
		},
	}
	if err := controllerutil.SetControllerReference(workload, deployment, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, r.Create(ctx, deployment)
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CustomDeployment{}).
		Owns(&appsv1.Deployment{}).
		Named("customdeployment").
		Complete(r)
}
