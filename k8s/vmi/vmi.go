package k8s

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
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

func WithUserData(userDataRef *corev1.LocalObjectReference) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		vmi, ok := o.(*kubevirtv1.VirtualMachineInstance)
		if !ok {
			return errors.New("not a instance of VirtualMachineInstance")
		}
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

func CreateIfNotExist(
	ctx context.Context,
	owner metav1.Object,
	c client.Client,
	opts ...k8s_object.ObjectOption,
) (*kubevirtv1.VirtualMachineInstance, error) {
	vmi := newDefaultVirtualMachineInstance()
	for _, fn := range opts {
		if err := fn(&vmi); err != nil {
			return nil, err
		}
	}
	if err := c.Create(ctx, &vmi); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := c.Get(ctx, client.ObjectKeyFromObject(&vmi), &vmi); err != nil {
				return nil, err
			}
			return &vmi, nil
		}
		return nil, fmt.Errorf(
			"unable to create VirtualMachineInstance %s: %w",
			k8s_object.ObjectName(&vmi.ObjectMeta),
			err,
		)
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", vmi.Namespace,
		"name", vmi.Name,
	)
	logger.Info("VirtualMachineInstance was created")

	return &vmi, nil
}
