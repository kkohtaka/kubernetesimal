package k8s

import (
	"context"
	_ "embed"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	DiskKeyForContainer = "containerdisk"
	DiskKeyForCloudInit = "cloudinitdisk"

	DefaultContainerDiskImage = "kubevirt/fedora-cloud-container-disk-demo"
)

var (
	DefaultResourceMemory = resource.MustParse("1024M")
)

func newDefaultVirtualMachineInstance() kubevirtv1.VirtualMachineInstance {
	return kubevirtv1.VirtualMachineInstance{
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Domain: kubevirtv1.DomainSpec{
				Devices: kubevirtv1.Devices{
					Disks: []kubevirtv1.Disk{
						{
							Name: DiskKeyForContainer,
							DiskDevice: kubevirtv1.DiskDevice{
								Disk: &kubevirtv1.DiskTarget{
									Bus: "virtio",
								},
							},
						},
						{
							Name: DiskKeyForCloudInit,
							DiskDevice: kubevirtv1.DiskDevice{
								Disk: &kubevirtv1.DiskTarget{
									Bus: "virtio",
								},
							},
						},
					},
					Interfaces: []kubevirtv1.Interface{
						*kubevirtv1.DefaultMasqueradeNetworkInterface(),
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
			Volumes: []kubevirtv1.Volume{
				{
					Name: DiskKeyForContainer,
					VolumeSource: kubevirtv1.VolumeSource{
						ContainerDisk: &kubevirtv1.ContainerDiskSource{
							Image: DefaultContainerDiskImage,
						},
					},
				},
			},
		},
	}
}

type VirtualMachineInstanceOption func(*kubevirtv1.VirtualMachineInstance) error

func WithUserData(userDataRef *corev1.LocalObjectReference) VirtualMachineInstanceOption {
	return func(vmi *kubevirtv1.VirtualMachineInstance) error {
		var volume *kubevirtv1.Volume
		for i := range vmi.Spec.Volumes {
			if vmi.Spec.Volumes[i].Name == DiskKeyForCloudInit {
				volume = &vmi.Spec.Volumes[i]
				break
			}
		}
		if volume == nil {
			vmi.Spec.Volumes = append(vmi.Spec.Volumes, kubevirtv1.Volume{
				Name: DiskKeyForCloudInit,
				VolumeSource: kubevirtv1.VolumeSource{
					CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
						UserDataSecretRef: userDataRef,
					},
				},
			})
		} else {
			volume.CloudInitNoCloud.UserDataSecretRef = userDataRef
		}
		return nil
	}
}

func WithVMIOwner(owner metav1.Object, scheme *runtime.Scheme) VirtualMachineInstanceOption {
	return func(vmi *kubevirtv1.VirtualMachineInstance) error {
		return ctrl.SetControllerReference(owner, vmi, scheme)
	}
}

func ReconcileVirtualMachineInstance(
	ctx context.Context,
	owner metav1.Object,
	c client.Client,
	meta *metav1.ObjectMeta,
	opts ...VirtualMachineInstanceOption,
) (*kubevirtv1.VirtualMachineInstance, error) {
	vmi := newDefaultVirtualMachineInstance()
	meta.DeepCopyInto(&vmi.ObjectMeta)
	opRes, err := ctrl.CreateOrUpdate(ctx, c, &vmi, func() error {
		for _, fn := range opts {
			if err := fn(&vmi); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create VirtualMachineInstance %s: %w", ObjectName(&vmi.ObjectMeta), err)
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", vmi.Namespace,
		"name", vmi.Name,
	)
	switch opRes {
	case controllerutil.OperationResultCreated:
		logger.Info("VirtualMachineInstance was created")
	case controllerutil.OperationResultUpdated:
		logger.Info("VirtualMachineInstance was updated")
	}

	return &vmi, nil
}
