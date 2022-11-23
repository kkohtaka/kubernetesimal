/*
MIT License

Copyright (c) 2022 Kazumasa Kohtaka

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
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
