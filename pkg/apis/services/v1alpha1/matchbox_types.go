/*
Copyright 2019 Kazumasa Kohtaka <kkohtaka@gmail.com>.

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

// MatchboxSpec defines the desired state of Matchbox
type MatchboxSpec struct {
}

// MatchboxStatus defines the observed state of Matchbox
type MatchboxStatus struct {
	Ready bool `json:"ready"`

	PacketDeviceRef PacketDeviceRef `json:"packetDeviceRef,omitempty"`
}

type PacketDeviceRef struct {
	Name string `json:"name,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Matchbox is the Schema for the matchboxes API
// +k8s:openapi-gen=true
type Matchbox struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MatchboxSpec   `json:"spec,omitempty"`
	Status MatchboxStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MatchboxList contains a list of Matchbox
type MatchboxList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Matchbox `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Matchbox{}, &MatchboxList{})
}
