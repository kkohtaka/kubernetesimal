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

// EtcdSpec defines the desired state of Etcd
type EtcdSpec struct {
	// Version is the desired version of the etcd cluster.
	Version *string `json:"version,omitempty"`

	// Replicas is the desired number of etcd replicas.
	Replicas *int32 `json:"replicas,omitempty"`
}

// EtcdStatus defines the observed state of Etcd
type EtcdStatus struct {
	// Phase indicates phase of the etcd cluster.
	//+kubebuilder:default=Creating
	Phase EtcdPhase `json:"phase"`

	// CACertificateRef is a reference to a Secret key that composes a CA certificate.
	CACertificateRef *corev1.SecretKeySelector `json:"caCertificateRef,omitempty"`
	// CAPrivateKeyRef is a reference to a Secret key that composes a CA private key.
	CAPrivateKeyRef *corev1.SecretKeySelector `json:"caPrivateKeyRef,omitempty"`
	// ClientCertificateRef is a reference to a Secret key that composes a Client certificate.
	ClientCertificateRef *corev1.SecretKeySelector `json:"clientCertificateRef,omitempty"`
	// ClientPrivateKeyRef is a reference to a Secret key that composes a Client private key.
	ClientPrivateKeyRef *corev1.SecretKeySelector `json:"clientPrivateKeyRef,omitempty"`
	// PeerCertificateRef is a reference to a Secret key that composes a certificate for peer communication.
	PeerCertificateRef *corev1.SecretKeySelector `json:"peerCertificateRef,omitempty"`
	// PeerPrivateKeyRef is a reference to a Secret key that composes a peer private key for peer communication.
	PeerPrivateKeyRef *corev1.SecretKeySelector `json:"peerPrivateKeyRef,omitempty"`
	// SSHPrivateKeyRef is a reference to a Secret key that composes an SSH private key.
	SSHPrivateKeyRef *corev1.SecretKeySelector `json:"sshPrivateKeyRef,omitempty"`
	// SSHPublicKeyRef is a reference to a Secret key that composes an SSH public key.
	SSHPublicKeyRef *corev1.SecretKeySelector `json:"sshPublicKeyRef,omitempty"`
	// UserDataRef is a reference to a Secret that contains a userdata used to start a virtual machine instance.
	UserDataRef *corev1.LocalObjectReference `json:"userDataRef,omitempty"`
	// VirtualMachineRef is a reference to a VirtualMachineInstance that composes an etcd node.
	VirtualMachineRef *corev1.LocalObjectReference `json:"virtualMachineRef,omitempty"`
	// ServiceRef is a reference to a Service of an etcd node.
	ServiceRef *corev1.LocalObjectReference `json:"serviceRef,omitempty"`
	// LastProvisionedTime is the timestamp when the controller probed an etcd node at the first time.
	LastProvisionedTime *metav1.Time `json:"lastProvisionedTime,omitempty"`
	// ProbedSinceTime is the timestamp when the controller probed an etcd node at the first time.
	ProbedSinceTime *metav1.Time `json:"probedSinceTime,omitempty"`

	// Replicas is the current number of etcd replicas.
	//+kubebuilder:default=0
	Replicas int32 `json:"replicas,omitempty"`
}

// EtcdPhase is a label for the phase of the etcd cluster at the current time.
//+kubebuilder:validation:Enum=Creating;Provisioned;Running;Deleting
type EtcdPhase string

const (
	// EtcdPhaseCreating means the etcd cluster is being created.
	EtcdPhaseCreating EtcdPhase = "Creating"
	// EtcdPhaseProvisioned means the etcd cluster was provisioned and wating to become running.
	EtcdPhaseProvisioned EtcdPhase = "Provisioned"
	// EtcdPhaseRunning means the etcd cluster is running.
	EtcdPhaseRunning EtcdPhase = "Running"
	// EtcdPhaseDeleting means the etcd cluster is running.
	EtcdPhaseDeleting EtcdPhase = "Deleting"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Desired Replicas",type=integer,JSONPath=`.spec.replicas`
//+kubebuilder:printcolumn:name="Current Replicas",type=integer,JSONPath=`.status.replicas`

// Etcd is the Schema for the etcds API
type Etcd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdSpec   `json:"spec,omitempty"`
	Status EtcdStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EtcdList contains a list of Etcd
type EtcdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Etcd `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Etcd{}, &EtcdList{})
}
