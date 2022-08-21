/*
Copyright 2021.

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

// EtcdNodeSetSpec defines the desired state of EtcdNodeSet
type EtcdNodeSetSpec struct {
	// Replicas is the number of desired replicas.
	// This is a pointer to distinguish between explicit zero and unspecified.
	// Defaults to 1.
	//+kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Template is the object that describes the EtcdNode that will be created if insufficient replicas are detected.
	Template EtcdNodeTemplateSpec `json:"template,omitempty"`
}

// EtcdNodeSetStatus defines the observed state of EtcdNodeSet
type EtcdNodeSetStatus struct {
	// Replicas is the most recently observed number of replicas.
	//+kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`

	// ActiveReplicas is the number of EtcdNodes targeted by this EtcdNodeSet.
	//+kubebuilder:validation:Minimum=0
	ActiveReplicas int32 `json:"activeReplicas,omitempty"`

	// ReadyReplicas is the number of EtcdNodes targeted by this EtcdNodeSet with a Ready Condition.
	//+kubebuilder:validation:Minimum=0
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// AvailableReplicas is the number of EtcdNodes targeted by this EtcdNodeSet
	//+kubebuilder:validation:Minimum=0
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed EtcdNodeSet.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
//+kubebuilder:printcolumn:name="Desired Replicas",type=integer,JSONPath=`.spec.replicas`
//+kubebuilder:printcolumn:name="Current Replicas",type=integer,priority=1,JSONPath=`.status.replicas`
//+kubebuilder:printcolumn:name="Active Replicas",type=integer,priority=1,JSONPath=`.status.activeReplicas`
//+kubebuilder:printcolumn:name="Ready Replicas",type=integer,JSONPath=`.status.readyReplicas`
//+kubebuilder:printcolumn:name="Available Replicas",type=integer,priority=1,JSONPath=`.status.availableReplicas`

// EtcdNodeSet is the Schema for the etcdnodesets API
type EtcdNodeSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdNodeSetSpec   `json:"spec,omitempty"`
	Status EtcdNodeSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EtcdNodeSetList contains a list of EtcdNodeSet
type EtcdNodeSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdNodeSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EtcdNodeSet{}, &EtcdNodeSetList{})
}
