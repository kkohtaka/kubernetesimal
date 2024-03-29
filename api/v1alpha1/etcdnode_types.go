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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EtcdNodeTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the EtcdNode.
	Spec EtcdNodeSpec `json:"spec,omitempty"`
}

// EtcdNodeSpec defines the desired state of EtcdNode
type EtcdNodeSpec struct {
	// Version is the desired version of the etcd cluster.
	Version string `json:"version"`

	// ImagePersistentVolumeClaimRef is a local reference to a PersistentVolumeClaim that is used as an ephemeral volume
	// to boot VirtualMachines.
	ImagePersistentVolumeClaimRef corev1.LocalObjectReference `json:"imagePersistentVolumeClaimRef"`

	// LoginPasswordSecretKeySelector is a selector for a Secret key that holds a password used as a login password of
	// virtual machines.
	LoginPasswordSecretKeySelector *corev1.SecretKeySelector `json:"loginPasswordSecretKeySelector,omitempty"`

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

	// AsFirstNode is whether the node is the first node of a cluster.
	AsFirstNode bool `json:"asFirstNode"`
}

// EtcdNodeStatus defines the observed state of EtcdNode
type EtcdNodeStatus struct {
	// Phase indicates phase of the etcd node.
	//+kubebuilder:default=Creating
	Phase EtcdNodePhase `json:"phase"`

	// UserDataRef is a reference to a Secret that contains a userdata used to start a virtual machine instance.
	UserDataRef *corev1.LocalObjectReference `json:"userDataRef,omitempty"`
	// VirtualMachineInstanceRef is a reference to a VirtualMachineInstance that composes an etcd node.
	VirtualMachineInstanceRef *corev1.LocalObjectReference `json:"virtualMachineInstanceRef,omitempty"`
	// PeerServiceRef is a reference to a Service of an etcd node.
	PeerServiceRef *corev1.LocalObjectReference `json:"peerServiceRef,omitempty"`

	// Conditions is a list of statuses respected to certain conditions.
	Conditions []EtcdNodeCondition `json:"conditions,omitempty"`
}

// EtcdNodePhase is a label for the phase of the etcd cluster at the current time.
// +kubebuilder:validation:Enum=Creating;Provisioned;Running;Deleting;Error
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
// +kubebuilder:validation:Enum=Ready;Provisioned;MemberFinalized
type EtcdNodeConditionType string

const (
	// EtcdNodeConditionTypeReady is a status respective to a node readiness.
	EtcdNodeConditionTypeReady EtcdNodeConditionType = "Ready"
	// EtcdNodeConditionTypeProvisioned is a status respective to a node provisioning.
	EtcdNodeConditionTypeProvisioned EtcdNodeConditionType = "Provisioned"
	// EtcdNodeConditionTypeMemberFinalized is a status representing a node as an etcd member was left from a cluster.
	EtcdNodeConditionTypeMemberFinalized EtcdNodeConditionType = "MemberFinalized"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`

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

func (status *EtcdNodeStatus) LastReadyProbeTime() *metav1.Time {
	for i := range status.Conditions {
		if status.Conditions[i].Type == EtcdNodeConditionTypeReady {
			return status.Conditions[i].LastProbeTime
		}
	}
	return nil
}

func (status *EtcdNodeStatus) IsProvisioned() bool {
	for i := range status.Conditions {
		if status.Conditions[i].Type == EtcdNodeConditionTypeProvisioned {
			return !status.Conditions[i].LastProbeTime.IsZero()
		}
	}
	return false
}

func (status *EtcdNodeStatus) IsReady() bool {
	for i := range status.Conditions {
		if status.Conditions[i].Type == EtcdNodeConditionTypeReady {
			return status.Conditions[i].Status == corev1.ConditionTrue
		}
	}
	return false
}

func (status *EtcdNodeStatus) IsReadyOnce() bool {
	for i := range status.Conditions {
		if status.Conditions[i].Type == EtcdNodeConditionTypeReady {
			return !status.Conditions[i].LastProbeTime.IsZero()
		}
	}
	return false
}

func (status *EtcdNodeStatus) ReadySinceTime() *metav1.Time {
	for i := range status.Conditions {
		if status.Conditions[i].Type == EtcdNodeConditionTypeReady {
			return status.Conditions[i].LastTransitionTime
		}
	}
	return nil
}

func (status *EtcdNodeStatus) IsMemberFinalized() bool {
	for i := range status.Conditions {
		if status.Conditions[i].Type == EtcdNodeConditionTypeMemberFinalized {
			return !status.Conditions[i].LastProbeTime.IsZero()
		}
	}
	return false
}

func (status *EtcdNodeStatus) WithReady(
	ready bool,
	message string,
) *EtcdNodeStatus {
	return status.WithStatusCondition(
		EtcdNodeConditionTypeReady,
		ready,
		message,
	)
}

func (status *EtcdNodeStatus) WithProvisioned(
	provisioned bool,
	message string,
) *EtcdNodeStatus {
	return status.WithStatusCondition(
		EtcdNodeConditionTypeProvisioned,
		provisioned,
		message,
	)
}

func (status *EtcdNodeStatus) WithMemberFinalized(
	leftFromCluster bool,
	message string,
) *EtcdNodeStatus {
	return status.WithStatusCondition(
		EtcdNodeConditionTypeMemberFinalized,
		leftFromCluster,
		message,
	)
}

func (status *EtcdNodeStatus) WithStatusCondition(
	conditionType EtcdNodeConditionType,
	ready bool,
	message string,
) *EtcdNodeStatus {
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
		EtcdNodeCondition{
			Type:               conditionType,
			Status:             condStatus,
			LastProbeTime:      lastProbeTime,
			LastTransitionTime: &now,
			Message:            message,
		},
	)
	return newStatus
}
