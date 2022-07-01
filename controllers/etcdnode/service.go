package etcdnode

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
	k8s_service "github.com/kkohtaka/kubernetesimal/k8s/service"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

const (
	serviceNameEtcd = "etcd"
	serviceNamePeer = "peer"
	serviceNameSSH  = "ssh"

	servicePortEtcd = 2379
	servicePortPeer = 2380
	servicePortSSH  = 22

	serviceContainerPortEtcd = 2379
	serviceContainerPortPeer = 2380
	serviceContainerPortSSH  = 22
)

func newPeerServiceName(obj client.Object) string {
	return obj.GetName()
}

func reconcilePeerService(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	obj client.Object,
	_ *kubernetesimalv1alpha1.EtcdNodeSpec,
	_ *kubernetesimalv1alpha1.EtcdNodeStatus,
) (*corev1.LocalObjectReference, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileService")
	defer span.End()

	if service, err := k8s_service.Reconcile(
		ctx,
		obj,
		c,
		newPeerServiceName(obj),
		obj.GetNamespace(),
		k8s_object.WithOwner(obj, scheme),
		k8s_service.WithType(corev1.ServiceTypeNodePort),
		k8s_service.WithPort(serviceNameEtcd, servicePortEtcd, serviceContainerPortEtcd),
		k8s_service.WithPort(serviceNamePeer, servicePortPeer, serviceContainerPortPeer),
		k8s_service.WithPort(serviceNameSSH, servicePortSSH, serviceContainerPortSSH),
		k8s_service.WithSelector("app.kubernetes.io/name", "virtualmachineimage"),
		k8s_service.WithSelector("app.kubernetes.io/instance", newVirtualMachineInstanceName(obj)),
		k8s_service.WithSelector("app.kubernetes.io/part-of", "etcd"),
	); err != nil {
		return nil, fmt.Errorf("unable to prepare a Service for an etcd member: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: service.Name,
		}, nil
	}
}
