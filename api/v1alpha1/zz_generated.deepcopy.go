//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ControllerTracing) DeepCopyInto(out *ControllerTracing) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ControllerTracing.
func (in *ControllerTracing) DeepCopy() *ControllerTracing {
	if in == nil {
		return nil
	}
	out := new(ControllerTracing)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Etcd) DeepCopyInto(out *Etcd) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Etcd.
func (in *Etcd) DeepCopy() *Etcd {
	if in == nil {
		return nil
	}
	out := new(Etcd)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Etcd) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdList) DeepCopyInto(out *EtcdList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Etcd, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdList.
func (in *EtcdList) DeepCopy() *EtcdList {
	if in == nil {
		return nil
	}
	out := new(EtcdList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *EtcdList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdSpec) DeepCopyInto(out *EtcdSpec) {
	*out = *in
	if in.Version != nil {
		in, out := &in.Version, &out.Version
		*out = new(string)
		**out = **in
	}
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdSpec.
func (in *EtcdSpec) DeepCopy() *EtcdSpec {
	if in == nil {
		return nil
	}
	out := new(EtcdSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EtcdStatus) DeepCopyInto(out *EtcdStatus) {
	*out = *in
	if in.CACertificateRef != nil {
		in, out := &in.CACertificateRef, &out.CACertificateRef
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	if in.CAPrivateKeyRef != nil {
		in, out := &in.CAPrivateKeyRef, &out.CAPrivateKeyRef
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	if in.ClientCertificateRef != nil {
		in, out := &in.ClientCertificateRef, &out.ClientCertificateRef
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	if in.ClientPrivateKeyRef != nil {
		in, out := &in.ClientPrivateKeyRef, &out.ClientPrivateKeyRef
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	if in.PeerCertificateRef != nil {
		in, out := &in.PeerCertificateRef, &out.PeerCertificateRef
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	if in.PeerPrivateKeyRef != nil {
		in, out := &in.PeerPrivateKeyRef, &out.PeerPrivateKeyRef
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	if in.SSHPrivateKeyRef != nil {
		in, out := &in.SSHPrivateKeyRef, &out.SSHPrivateKeyRef
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	if in.SSHPublicKeyRef != nil {
		in, out := &in.SSHPublicKeyRef, &out.SSHPublicKeyRef
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	if in.UserDataRef != nil {
		in, out := &in.UserDataRef, &out.UserDataRef
		*out = new(v1.LocalObjectReference)
		**out = **in
	}
	if in.VirtualMachineRef != nil {
		in, out := &in.VirtualMachineRef, &out.VirtualMachineRef
		*out = new(v1.LocalObjectReference)
		**out = **in
	}
	if in.ServiceRef != nil {
		in, out := &in.ServiceRef, &out.ServiceRef
		*out = new(v1.LocalObjectReference)
		**out = **in
	}
	if in.LastProvisionedTime != nil {
		in, out := &in.LastProvisionedTime, &out.LastProvisionedTime
		*out = (*in).DeepCopy()
	}
	if in.ProbedSinceTime != nil {
		in, out := &in.ProbedSinceTime, &out.ProbedSinceTime
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EtcdStatus.
func (in *EtcdStatus) DeepCopy() *EtcdStatus {
	if in == nil {
		return nil
	}
	out := new(EtcdStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KubernetesimalConfig) DeepCopyInto(out *KubernetesimalConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.ControllerManagerConfigurationSpec.DeepCopyInto(&out.ControllerManagerConfigurationSpec)
	out.Tracing = in.Tracing
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KubernetesimalConfig.
func (in *KubernetesimalConfig) DeepCopy() *KubernetesimalConfig {
	if in == nil {
		return nil
	}
	out := new(KubernetesimalConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *KubernetesimalConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
