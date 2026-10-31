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
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
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

const readyConditionType = "Ready"

// +kubebuilder:rbac:groups=workloads.example.com,resources=customdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=workloads.example.com,resources=customdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=workloads.example.com,resources=customdeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch

// Reconcile creates or updates the owned Deployment and publishes status; initial replicas are never reapplied.
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
			conflictErr := fmt.Errorf("deployment %s is not controlled by CustomDeployment %s", req.NamespacedName, workload.Name)
			if err := r.updateStatus(ctx, workload, nil, metav1.Condition{
				Type: readyConditionType, Status: metav1.ConditionFalse, Reason: "OwnershipConflict",
				Message: fmt.Sprintf("Deployment %s is not owned by this CustomDeployment", req.NamespacedName),
			}); err != nil {
				return ctrl.Result{}, fmt.Errorf("%w (status update: %v)", conflictErr, err)
			}
			return ctrl.Result{}, conflictErr
		}
		containers := deployment.Spec.Template.Spec.Containers
		if len(containers) != 1 {
			return ctrl.Result{}, fmt.Errorf("deployment %s must have exactly one container", req.NamespacedName)
		}
		container := &containers[0]
		if container.Image != workload.Spec.Image || !equality.Semantic.DeepEqual(container.Resources, workload.Spec.Resources) {
			container.Image = workload.Spec.Image
			container.Resources = workload.Spec.Resources
			if err := r.Update(ctx, deployment); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, r.updateStatus(ctx, workload, deployment, deploymentReadyCondition(deployment))
	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	labels := map[string]string{"workloads.example.com/customdeployment-uid": string(workload.UID)}
	replicas := min(max(workload.Spec.Replicas, workload.Spec.MinReplicas), workload.Spec.MaxReplicas)
	deployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: workload.Name, Namespace: workload.Namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: new(replicas),
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
	if err := r.Create(ctx, deployment); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, r.updateStatus(ctx, workload, deployment, deploymentReadyCondition(deployment))
}

func deploymentReadyCondition(deployment *appsv1.Deployment) metav1.Condition {
	desired := *deployment.Spec.Replicas
	if deployment.Status.ObservedGeneration < deployment.Generation {
		return metav1.Condition{
			Type: readyConditionType, Status: metav1.ConditionFalse, Reason: "RolloutInProgress",
			Message: fmt.Sprintf("Deployment has not observed generation %d", deployment.Generation),
		}
	}
	if deployment.Status.UpdatedReplicas != desired ||
		deployment.Status.Replicas != desired ||
		deployment.Status.ReadyReplicas != desired ||
		deployment.Status.AvailableReplicas != desired {
		return metav1.Condition{
			Type: readyConditionType, Status: metav1.ConditionFalse, Reason: "RolloutInProgress",
			Message: fmt.Sprintf(
				"Deployment has %d updated, %d ready, and %d available replicas; want %d",
				deployment.Status.UpdatedReplicas, deployment.Status.ReadyReplicas, deployment.Status.AvailableReplicas, desired,
			),
		}
	}
	return metav1.Condition{
		Type: readyConditionType, Status: metav1.ConditionTrue, Reason: "RolloutComplete",
		Message: fmt.Sprintf("Deployment observed generation %d with %d ready replicas", deployment.Generation, desired),
	}
}

func (r *CustomDeploymentReconciler) updateStatus(
	ctx context.Context,
	workload *workloadsv1alpha1.CustomDeployment,
	deployment *appsv1.Deployment,
	condition metav1.Condition,
) error {
	status := *workload.Status.DeepCopy()
	status.ObservedGeneration = workload.Generation
	status.DesiredReplicas = 0
	status.ReadyReplicas = 0
	if deployment != nil {
		status.DesiredReplicas = *deployment.Spec.Replicas
		status.ReadyReplicas = deployment.Status.ReadyReplicas
	}
	condition.ObservedGeneration = workload.Generation
	apimeta.SetStatusCondition(&status.Conditions, condition)
	if equality.Semantic.DeepEqual(status, workload.Status) {
		return nil
	}
	key := client.ObjectKeyFromObject(workload)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &workloadsv1alpha1.CustomDeployment{}
		if err := r.Get(ctx, key, latest); err != nil {
			return err
		}
		if equality.Semantic.DeepEqual(latest.Status, status) {
			return nil
		}
		latest.Status = status
		return r.Status().Update(ctx, latest)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CustomDeployment{}).
		Owns(&appsv1.Deployment{}).
		Named("customdeployment").
		Complete(r)
}
