/*
Copyright 2023.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	YttSourceKind = "YttSourceKind"
)

// YttSourceSpec defines the desired state of YttSource
type YttSourceSpec struct {
	// Namespace of the referenced resource.
	// Namespace can be left empty. In such a case, namespace will
	// be implicit set to cluster's namespace.
	Namespace string `json:"namespace"`

	// Name of the rreferenced resource.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Kind of the resource. Supported kinds are:
	// - flux GitRepository;OCIRepository;Bucket
	// - ConfigMap/Secret (which will be mounted as volume)
	// +kubebuilder:validation:Enum=GitRepository;OCIRepository;Bucket;ConfigMap;Secret
	Kind string `json:"kind"`

	// Path to the directory containing the kustomization.yaml file, or the
	// set of plain YAMLs a kustomization.yaml should be generated for.
	// Defaults to 'None', which translates to the root path of the SourceRef.
	// +optional
	Path string `json:"path,omitempty"`
}

// YttSourceStatus defines the observed state of YttSource
type YttSourceStatus struct {
	// Resources contains the output of YTT, so the
	// resources to be deployed
	Resources string `json:"resources,omitempty"`

	// FailureMessage provides more information about the error.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// YttSource is the Schema for the yttsources API
type YttSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   YttSourceSpec   `json:"spec,omitempty"`
	Status YttSourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// YttSourceList contains a list of YttSource
type YttSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []YttSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&YttSource{}, &YttSourceList{})
}
