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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EtcdSpec defines the desired state of Etcd
type EtcdSpec struct {
	// Version is the desired version of the etcd cluster.
	Version *string `json:"version,omitempty"`

	// Replicas is the desired number of etcd replicas.
	//+kubebuilder:validation:Minimum=0
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
	// ServiceRef is a reference to a Service of an etcd cluster.
	ServiceRef *corev1.LocalObjectReference `json:"serviceRef,omitempty"`
	// EndpointSliceRef is a reference to an EndpointSlice of an etcd cluster.
	EndpointSliceRef *corev1.LocalObjectReference `json:"endpointSliceRef,omitempty"`
	// Nodes is a list of references of EtcdNodes that composes the etcd cluster.
	NodeRefs []*corev1.LocalObjectReference `json:"nodeRefs,omitempty"`

	// Replicas is the current number of etcd replicas.
	//+kubebuilder:default=0
	Replicas int32 `json:"replicas,omitempty"`

	// Conditions is a list of statuses respected to certain conditions.
	Conditions []EtcdCondition `json:"conditions,omitempty"`
}

// EtcdPhase is a label for the phase of the etcd cluster at the current time.
//+kubebuilder:validation:Enum=Creating;Running;Deleting;Error
type EtcdPhase string

const (
	// EtcdPhaseCreating means the etcd cluster is being created.
	EtcdPhaseCreating EtcdPhase = "Creating"
	// EtcdPhaseRunning means the etcd cluster is running.
	EtcdPhaseRunning EtcdPhase = "Running"
	// EtcdPhaseDeleting means the etcd cluster is being deleted.
	EtcdPhaseDeleting EtcdPhase = "Deleting"
	// EtcdPhaseError means the etcd cluster is in error state.
	EtcdPhaseError EtcdPhase = "Error"
)

// EtcdCondition defines a status respected to a certain condition.
type EtcdCondition struct {
	// Type is the type of the condition.
	Type EtcdConditionType `json:"type"`
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

// EtcdConditionType represents a type of condition.
//+kubebuilder:validation:Enum=Ready;MembersHealthy
type EtcdConditionType string

const (
	// EtcdConditionTypeReady is a status respective to a cluster readiness.
	EtcdConditionTypeReady EtcdConditionType = "Ready"

	// EtcdConditionTypeMembersHealthy indicates whether all EtcdNodes are registered successfully and healthy.
	EtcdConditionTypeMembersHealthy EtcdConditionType = "MembersHealthy"
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

func (status *EtcdStatus) LastReadyProbeTime() *metav1.Time {
	for i := range status.Conditions {
		if status.Conditions[i].Type == EtcdConditionTypeReady {
			return status.Conditions[i].LastProbeTime
		}
	}
	return nil
}

func (status *EtcdStatus) IsReady() bool {
	for i := range status.Conditions {
		if status.Conditions[i].Type == EtcdConditionTypeReady {
			return status.Conditions[i].Status == corev1.ConditionTrue
		}
	}
	return false
}

func (status *EtcdStatus) IsReadyOnce() bool {
	for i := range status.Conditions {
		if status.Conditions[i].Type == EtcdConditionTypeReady {
			return !status.Conditions[i].LastProbeTime.IsZero()
		}
	}
	return false
}

func (status *EtcdStatus) AreMembersHealthy() bool {
	for i := range status.Conditions {
		if status.Conditions[i].Type == EtcdConditionTypeMembersHealthy {
			return status.Conditions[i].Status == corev1.ConditionTrue
		}
	}
	return false
}

func (status *EtcdStatus) WithReady(
	ready bool,
	message string,
) *EtcdStatus {
	return status.WithStatusCondition(
		EtcdConditionTypeReady,
		ready,
		message,
	)
}

func (status *EtcdStatus) WithMembersHealthy(
	ready bool,
	message string,
) *EtcdStatus {
	return status.WithStatusCondition(
		EtcdConditionTypeMembersHealthy,
		ready,
		message,
	)
}

func (status *EtcdStatus) WithStatusCondition(
	conditionType EtcdConditionType,
	ready bool,
	message string,
) *EtcdStatus {
	newStatus := status.DeepCopy()
	now := metav1.NewTime(time.Now())
	condStatus := corev1.ConditionFalse
	if ready {
		condStatus = corev1.ConditionTrue
	}
	for i := range newStatus.Conditions {
		if newStatus.Conditions[i].Type == conditionType {
			if newStatus.Conditions[i].Status != condStatus {
				newStatus.Conditions[i].LastTransitionTime = &now
			}
			if ready {
				newStatus.Conditions[i].LastProbeTime = &now
			}
			newStatus.Conditions[i].Status = condStatus
			newStatus.Conditions[i].Message = message
			return newStatus
		}
	}
	var lastProbeTime *metav1.Time
	if ready {
		lastProbeTime = &now
	}
	newStatus.Conditions = append(
		newStatus.Conditions,
		EtcdCondition{
			Type:               conditionType,
			Status:             condStatus,
			LastProbeTime:      lastProbeTime,
			LastTransitionTime: &now,
			Message:            message,
		},
	)
	return newStatus
}
