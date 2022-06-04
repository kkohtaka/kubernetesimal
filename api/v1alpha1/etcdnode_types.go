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

	// Conditions is a list of statuses respected to certain conditions.
	Conditions []EtcdNodeCondition `json:"conditions,omitempty"`
}

// EtcdNodePhase is a label for the phase of the etcd cluster at the current time.
//+kubebuilder:validation:Enum=Creating;Provisioned;Running;Deleting;Error
type EtcdNodePhase string

const (
	// EtcdNodePhaseCreating means the etcd node is being created.
	EtcdNodePhaseCreating EtcdNodePhase = "Creating"
	// EtcdNodePhaseProvisioned means the etcd node was provisioned and waiting to become running.
	EtcdNodePhaseProvisioned EtcdNodePhase = "Provisioned"
	// EtcdNodePhaseRunning means the etcd node is running.
	EtcdNodePhaseRunning EtcdNodePhase = "Running"
	// EtcdNodePhaseDeleting means the etcd node is being deleted.
	EtcdNodePhaseDeleting EtcdNodePhase = "Deleting"
	// EtcdNodePhaseError means the etcd node is in error state.
	EtcdNodePhaseError EtcdNodePhase = "Error"
)

// EtcdNodeCondition defines a status respected to a certain condition.
type EtcdNodeCondition struct {
	// Type is the type of the condition.
	Type EtcdNodeConditionType `json:"type"`
	// Status is the status of the condition.
	Status corev1.ConditionStatus `json:"status"`
	// Last time we probed the condition.
	LastProbeTime *metav1.Time `json:"lastProbeTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// Unique, one-word, CamelCase reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition.
	Message string `json:"message,omitempty"`
}

// EtcdNodeConditionType represents a type of condition.
//+kubebuilder:validation:Enum=Ready;Provisioned
type EtcdNodeConditionType string

const (
	// EtcdNodeConditionTypeReady is a status respective to a node readiness.
	EtcdNodeConditionTypeReady EtcdNodeConditionType = "Ready"
	// EtcdNodeConditionTypeProvisioned is a status respective to a node provisioning.
	EtcdNodeConditionTypeProvisioned EtcdNodeConditionType = "Provisioned"
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
