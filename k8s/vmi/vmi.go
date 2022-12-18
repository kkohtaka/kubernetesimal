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

package k8s

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
)

const (
	DiskKeyForBoot      = "boot"
	DiskKeyForCloudInit = "cloud-init"
)

var (
	DefaultResourceMemory = resource.MustParse("1024M")
)

func newDefaultVirtualMachineInstance() kubevirtv1.VirtualMachineInstance {
	return kubevirtv1.VirtualMachineInstance{
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Domain: kubevirtv1.DomainSpec{
				Devices: kubevirtv1.Devices{
					Interfaces: []kubevirtv1.Interface{
						*kubevirtv1.DefaultBridgeNetworkInterface(),
					},
				},
				Resources: kubevirtv1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: DefaultResourceMemory,
					},
				},
			},
			Networks: []kubevirtv1.Network{
				*kubevirtv1.DefaultPodNetwork(),
			},
		},
	}
}

func WithEphemeralVolumeSource(claimName string) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		vmi, ok := o.(*kubevirtv1.VirtualMachineInstance)
		if !ok {
			return errors.New("not a instance of VirtualMachineInstance")
		}
		for _, v := range vmi.Spec.Volumes {
			if v.Name == DiskKeyForBoot {
				return fmt.Errorf("boot volume is already set")
			}
		}
		vmi.Spec.Volumes = append(vmi.Spec.Volumes, kubevirtv1.Volume{
			Name: DiskKeyForBoot,
			VolumeSource: kubevirtv1.VolumeSource{
				Ephemeral: &kubevirtv1.EphemeralVolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
						ReadOnly:  true,
					},
				},
			},
		})
		vmi.Spec.Domain.Devices.Disks = append(vmi.Spec.Domain.Devices.Disks, kubevirtv1.Disk{
			Name: DiskKeyForBoot,
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: kubevirtv1.DiskBusVirtio,
				},
			},
		})
		return nil
	}
}

func WithUserDataSecret(userDataRef *corev1.LocalObjectReference) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		vmi, ok := o.(*kubevirtv1.VirtualMachineInstance)
		if !ok {
			return errors.New("not a instance of VirtualMachineInstance")
		}
		for i := range vmi.Spec.Volumes {
			if vmi.Spec.Volumes[i].Name == DiskKeyForCloudInit {
				return fmt.Errorf("cloud-init data is already set")
			}
		}
		vmi.Spec.Volumes = append(vmi.Spec.Volumes, kubevirtv1.Volume{
			Name: DiskKeyForCloudInit,
			VolumeSource: kubevirtv1.VolumeSource{
				CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
					UserDataSecretRef: userDataRef,
				},
			},
		})
		vmi.Spec.Domain.Devices.Disks = append(vmi.Spec.Domain.Devices.Disks, kubevirtv1.Disk{
			Name: DiskKeyForCloudInit,
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: kubevirtv1.DiskBusVirtio,
				},
			},
		})
		return nil
	}
}

func WithReadinessTCPProbe(tcpAction *corev1.TCPSocketAction) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		vmi, ok := o.(*kubevirtv1.VirtualMachineInstance)
		if !ok {
			return errors.New("not a instance of VirtualMachineInstance")
		}
		vmi.Spec.ReadinessProbe = &kubevirtv1.Probe{
			Handler: kubevirtv1.Handler{
				TCPSocket: tcpAction,
			},
		}
		return nil
	}
}

func CreateOnlyIfNotExist(
	ctx context.Context,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (controllerutil.OperationResult, *kubevirtv1.VirtualMachineInstance, error) {
	vmi := newDefaultVirtualMachineInstance()
	vmi.Name = name
	vmi.Namespace = namespace

	if err := c.Get(ctx, client.ObjectKeyFromObject(&vmi), &vmi); err != nil {
		if apierrors.IsNotFound(err) {
			return Reconcile(ctx, c, name, namespace, opts...)
		} else {
			return controllerutil.OperationResultNone, nil, err
		}
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", vmi.Namespace,
		"name", vmi.Name,
	)
	logger.V(4).Info("VirtualMachineInstance already exists")

	return controllerutil.OperationResultNone, &vmi, nil
}

func Reconcile(
	ctx context.Context,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (controllerutil.OperationResult, *kubevirtv1.VirtualMachineInstance, error) {
	vmi := newDefaultVirtualMachineInstance()
	vmi.Name = name
	vmi.Namespace = namespace

	opRes, err := ctrl.CreateOrUpdate(ctx, c, &vmi, func() error {
		for _, fn := range opts {
			if err := fn(&vmi); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return controllerutil.OperationResultNone, nil, fmt.Errorf(
			"unable to create or update VirtualMachineInstance %s: %w",
			k8s_object.ObjectName(&vmi.ObjectMeta),
			err,
		)
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", vmi.Namespace,
		"name", vmi.Name,
	)
	switch opRes {
	case controllerutil.OperationResultCreated:
		logger.Info("VirtualMachineInstance was created.")
	case controllerutil.OperationResultUpdated:
		logger.Info("VirtualMachineInstance was updated.")
	case controllerutil.OperationResultNone:
		logger.V(4).Info("VirtualMachineInstance was unchanged.")
	}

	return opRes, &vmi, nil
}
