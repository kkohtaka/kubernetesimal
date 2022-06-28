package etcd

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
	k8s_service "github.com/kkohtaka/kubernetesimal/k8s/service"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

const (
	ServiceNameEtcd = "etcd"

	ServicePortEtcd = 2379

	ServiceContainerPortEtcd = 2379
)

func newServiceName(e metav1.Object) string {
	return e.GetName()
}

func reconcileService(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	e metav1.Object,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.LocalObjectReference, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileService")
	defer span.End()

	if service, err := k8s_service.Reconcile(
		ctx,
		e,
		c,
		newServiceName(e),
		e.GetNamespace(),
		k8s_object.WithOwner(e, scheme),
		k8s_service.WithType(corev1.ServiceTypeNodePort),
		k8s_service.WithPort(ServiceNameEtcd, ServicePortEtcd, ServiceContainerPortEtcd),
	); err != nil {
		return nil, fmt.Errorf("unable to prepare a Service for an etcd member: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: service.Name,
		}, nil
	}
}
