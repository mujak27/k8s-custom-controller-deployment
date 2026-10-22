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
type CustomDeploymentSpec struct {
	// Image is the image for the single workload container.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Replicas is the initial replica count, used only when creating the Deployment.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas int32 `json:"replicas"`

	// Resources configures the single workload container's requests and limits.
	Resources corev1.ResourceRequirements `json:"resources"`
}

// CustomDeploymentStatus defines the observed state of CustomDeployment.
type CustomDeploymentStatus struct {
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
