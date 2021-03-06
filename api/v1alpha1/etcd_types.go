/*
Copyright 2020 Kazumasa Kohtaka <kkohtaka@gmail.com>.

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

// EtcdSpec defines the desired state of Etcd
type EtcdSpec struct {
	// Version is the desired version of the etcd cluster.
	Version string `json:"version"`
}

// EtcdStatus defines the observed state of Etcd
type EtcdStatus struct {
	// Phase indicates phase of the etcd cluster.
	Phase EtcdPhase `json:"phase"`

	// VirtualMachineRef is a namespaced name of the virtual machine that composes a member of the etcd cluster.
	VirtualMachineRef string `json:"virtualMachineRef,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Running

// EtcdPhase is a label for the phase of the etcd cluster at the current time.
type EtcdPhase string

const (
	// EtcdPhasePending means the etcd cluster is wating to become running.
	EtcdPhasePending EtcdPhase = "Pending"
	// EtcdPhaseRunning means the etcd cluster is running.
	EtcdPhaseRunning EtcdPhase = "Running"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Etcd is the Schema for the etcds API
type Etcd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdSpec   `json:"spec,omitempty"`
	Status EtcdStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EtcdList contains a list of Etcd
type EtcdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Etcd `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Etcd{}, &EtcdList{})
}
