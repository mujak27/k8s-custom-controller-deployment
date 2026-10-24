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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CustomDeploymentSpec defines the desired state of CustomDeployment
// +kubebuilder:validation:XValidation:rule="self.maxReplicas >= self.minReplicas",message="maxReplicas must be greater than or equal to minReplicas"
type CustomDeploymentSpec struct {
	// Image is the image for the single workload container.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Replicas is the initial replica count, used only when creating the Deployment.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="replicas is immutable"
	// +optional
	Replicas int32 `json:"replicas"`

	// MinReplicas is the lower bound for autoscaling.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the upper bound for autoscaling.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// Resources configures the single workload container's requests and limits.
	// +kubebuilder:validation:XValidation:rule="has(self.requests) && 'cpu' in self.requests",message="resources.requests.cpu is required"
	// +kubebuilder:validation:XValidation:rule="has(self.requests) && 'memory' in self.requests",message="resources.requests.memory is required"
	// +kubebuilder:validation:XValidation:rule="!has(self.requests) || !('cpu' in self.requests) || quantity(string(self.requests['cpu'])).compareTo(quantity('0')) > 0",message="resources.requests.cpu must be positive"
	// +kubebuilder:validation:XValidation:rule="!has(self.requests) || !('memory' in self.requests) || quantity(string(self.requests['memory'])).compareTo(quantity('0')) > 0",message="resources.requests.memory must be positive"
	// +kubebuilder:validation:XValidation:rule="!has(self.limits) || !('cpu' in self.limits) || quantity(string(self.limits['cpu'])).compareTo(quantity('0')) > 0",message="resources.limits.cpu must be positive"
	// +kubebuilder:validation:XValidation:rule="!has(self.limits) || !('memory' in self.limits) || quantity(string(self.limits['memory'])).compareTo(quantity('0')) > 0",message="resources.limits.memory must be positive"
	// +kubebuilder:validation:XValidation:rule="!has(self.limits) || !('cpu' in self.limits) || !has(self.requests) || !('cpu' in self.requests) || quantity(string(self.limits['cpu'])).compareTo(quantity(string(self.requests['cpu']))) >= 0",message="resources.limits.cpu must be greater than or equal to the CPU request"
	// +kubebuilder:validation:XValidation:rule="!has(self.limits) || !('memory' in self.limits) || !has(self.requests) || !('memory' in self.requests) || quantity(string(self.limits['memory'])).compareTo(quantity(string(self.requests['memory']))) >= 0",message="resources.limits.memory must be greater than or equal to the memory request"
	Resources corev1.ResourceRequirements `json:"resources"`
}

// CustomDeploymentStatus defines the observed state of CustomDeployment.
type CustomDeploymentStatus struct {
	// ObservedGeneration is the CustomDeployment generation processed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// DesiredReplicas is the current desired count on the owned Deployment.
	// +optional
	DesiredReplicas int32 `json:"desiredReplicas,omitempty"`

	// ReadyReplicas is the current ready count on the owned Deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Conditions describe workload readiness.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// CustomDeployment is the Schema for the customdeployments API
type CustomDeployment struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CustomDeployment
	// +required
	Spec CustomDeploymentSpec `json:"spec"`

	// status defines the observed state of CustomDeployment
	// +optional
	Status CustomDeploymentStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// CustomDeploymentList contains a list of CustomDeployment
type CustomDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CustomDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &CustomDeployment{}, &CustomDeploymentList{})
		return nil
	})
}
