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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EtcdNodeSpec defines the desired state of EtcdNode
type EtcdNodeSpec struct {
	// Version is the desired version of the etcd cluster.
	Version string `json:"version"`

	// CACertificateRef is a reference to a Secret key that composes a CA certificate.
	CACertificateRef corev1.SecretKeySelector `json:"caCertificateRef"`
	// CAPrivateKeyRef is a reference to a Secret key that composes a CA private key.
	CAPrivateKeyRef corev1.SecretKeySelector `json:"caPrivateKeyRef"`

	// ClientCertificateRef is a reference to a Secret key that composes a Client certificate.
	ClientCertificateRef corev1.SecretKeySelector `json:"clientCertificateRef,omitempty"`
	// ClientPrivateKeyRef is a reference to a Secret key that composes a Client private key.
	ClientPrivateKeyRef corev1.SecretKeySelector `json:"clientPrivateKeyRef,omitempty"`

	// SSHPrivateKeyRef is a reference to a Secret key that composes an SSH private key.
	SSHPrivateKeyRef corev1.SecretKeySelector `json:"sshPrivateKeyRef"`
	// SSHPublicKeyRef is a reference to a Secret key that composes an SSH public key.
	SSHPublicKeyRef corev1.SecretKeySelector `json:"sshPublicKeyRef"`

	// ServiceRef is a reference to a Service of an etcd cluster.
	ServiceRef corev1.LocalObjectReference `json:"serviceRef"`
}

// EtcdNodeStatus defines the observed state of EtcdNode
type EtcdNodeStatus struct {
	// Phase indicates phase of the etcd node.
	//+kubebuilder:default=Creating
	Phase EtcdNodePhase `json:"phase"`

	// UserDataRef is a reference to a Secret that contains a userdata used to start a virtual machine instance.
	UserDataRef *corev1.LocalObjectReference `json:"userDataRef,omitempty"`
	// VirtualMachineRef is a reference to a VirtualMachineInstance that composes an etcd node.
	VirtualMachineRef *corev1.LocalObjectReference `json:"virtualMachineRef,omitempty"`
	// PeerServiceRef is a reference to a Service of an etcd node.
	PeerServiceRef *corev1.LocalObjectReference `json:"peerServiceRef,omitempty"`

	// LastProvisionedTime is the timestamp when the controller probed an etcd node at the first time.
	LastProvisionedTime *metav1.Time `json:"lastProvisionedTime,omitempty"`
	// ProbedSinceTime is the timestamp when the controller probed an etcd node at the first time.
	ProbedSinceTime *metav1.Time `json:"probedSinceTime,omitempty"`
}

// EtcdNodePhase is a label for the phase of the etcd cluster at the current time.
//+kubebuilder:validation:Enum=Creating;Provisioned;Running;Deleting
type EtcdNodePhase string

const (
	// EtcdNodePhaseCreating means the etcd cluster is being created.
	EtcdNodePhaseCreating EtcdNodePhase = "Creating"
	// EtcdNodePhaseProvisioned means the etcd cluster was provisioned and wating to become running.
	EtcdNodePhaseProvisioned EtcdNodePhase = "Provisioned"
	// EtcdNodePhaseRunning means the etcd cluster is running.
	EtcdNodePhaseRunning EtcdNodePhase = "Running"
	// EtcdNodePhaseDeleting means the etcd cluster is running.
	EtcdNodePhaseDeleting EtcdNodePhase = "Deleting"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// EtcdNode is the Schema for the etcd nodes API
type EtcdNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdNodeSpec   `json:"spec,omitempty"`
	Status EtcdNodeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EtcdNodeList contains a list of EtcdNode
type EtcdNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EtcdNode{}, &EtcdNodeList{})
}
