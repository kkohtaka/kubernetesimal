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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EtcdNodeDeploymentSpec defines the desired state of EtcdNodeDeployment
type EtcdNodeDeploymentSpec struct {
	// Replicas is the number of desired replicas.
	// This is a pointer to distinguish between explicit zero and unspecified.
	// Defaults to 1.
	//+kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Template is the object that describes the EtcdNode that will be created if insufficient replicas are detected.
	Template EtcdNodeTemplateSpec `json:"template,omitempty"`

	// Rolling update config params. Present only if DeploymentStrategyType = RollingUpdate.
	RollingUpdate RollingUpdateEtcdNodeDeployment `json:"rollingUpdate,omitempty"`

	// The number of old EtcdNodeSets to retain to allow rollback.
	// This is a pointer to distinguish between explicit zero and not specified.
	// This is set to the max value of int32 (i.e. 2147483647) by default, which means
	// "retaining all old EtcdNodeSets".
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// RollingUpdateEtcdNodeDeployment is the spec to control the desired behavior of rolling update.
type RollingUpdateEtcdNodeDeployment struct {
	// The maximum number of pods that can be unavailable during the update.
	// Value can be an absolute number (ex: 5) or a percentage of total pods at the start of update (ex: 10%).
	// Absolute number is calculated from percentage by rounding down.
	// This can not be 0 if MaxSurge is 0.
	// By default, a fixed value of 1 is used.
	// Example: when this is set to 30%, the old RC can be scaled down by 30%
	// immediately when the rolling update starts. Once new pods are ready, old RC
	// can be scaled down further, followed by scaling up the new RC, ensuring
	// that at least 70% of original number of pods are available at all times
	// during the update.
	//+kubebuilder:default="25%"
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// The maximum number of pods that can be scheduled above the original number of
	// pods.
	// Value can be an absolute number (ex: 5) or a percentage of total pods at
	// the start of the update (ex: 10%). This can not be 0 if MaxUnavailable is 0.
	// Absolute number is calculated from percentage by rounding up.
	// By default, a value of 1 is used.
	// Example: when this is set to 30%, the new RC can be scaled up by 30%
	// immediately when the rolling update starts. Once old pods have been killed,
	// new RC can be scaled up further, ensuring that total number of pods running
	// at any time during the update is at most 130% of original pods.
	//+kubebuilder:default="25%"
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
}

// EtcdNodeDeploymentStatus defines the observed state of EtcdNodeDeployment
type EtcdNodeDeploymentStatus struct {
	// Replicas is the most recently observed number of replicas.
	//+kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas,omitempty"`

	// UpdatedReplicas
	//+kubebuilder:validation:Minimum=0
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// ReadyReplicas is the number of EtcdNodes targeted by this EtcdNodeSet with a Ready Condition.
	//+kubebuilder:validation:Minimum=0
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// AvailableReplicas
	//+kubebuilder:validation:Minimum=0
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// UnavailableReplicas
	//+kubebuilder:validation:Minimum=0
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed EtcdNodeSet.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Count of hash collisions for the EtcdNodeDeployment.
	// The EtcdNodeDeployment controller uses this field as a collision avoidance mechanism when it needs to create the
	// name for the newest EtcdNodeSet.
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// Revision
	//+kubebuilder:default=0
	Revision *int64 `json:"revision,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
//+kubebuilder:printcolumn:name="Desired Replicas",type=integer,JSONPath=`.spec.replicas`
//+kubebuilder:printcolumn:name="Current Replicas",type=integer,priority=1,JSONPath=`.status.replicas`
//+kubebuilder:printcolumn:name="Updated Replicas",type=integer,priority=1,JSONPath=`.status.updatedReplicas`
//+kubebuilder:printcolumn:name="Ready Replicas",type=integer,JSONPath=`.status.readyReplicas`
//+kubebuilder:printcolumn:name="Available Replicas",type=integer,priority=1,JSONPath=`.status.availableReplicas`
//+kubebuilder:printcolumn:name="Unavailable Replicas",type=integer,priority=1,JSONPath=`.status.unavailableReplicas`

// EtcdNodeDeployment is the Schema for the etcdnodedeployments API
type EtcdNodeDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdNodeDeploymentSpec   `json:"spec,omitempty"`
	Status EtcdNodeDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EtcdNodeDeploymentList contains a list of EtcdNodeDeployment
type EtcdNodeDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdNodeDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EtcdNodeDeployment{}, &EtcdNodeDeploymentList{})
}
