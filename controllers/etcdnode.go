package controllers

import (
	"context"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/observerbility/tracing"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func finalizeEtcdNodes(
	ctx context.Context,
	c client.Client,
	e metav1.Object,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizeEtcdNode")
	defer span.End()

	if len(status.NodeRefs) == 0 {
		return status, nil
	}
	for _, ref := range status.NodeRefs {
		if err := finalizeEtcdNode(ctx, c, e.GetNamespace(), ref.Name); err != nil {
			return status, err
		}
	}
	status.NodeRefs = nil
	log.FromContext(ctx).Info("EtcdNodes were finalized.")
	return status, nil
}

func finalizeEtcdNode(
	ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues(
		"object", name,
		"resource", "kubernetesimalv1alpha1.EtcdNode",
	))
	return finalizeObject(ctx, client, namespace, name, &kubernetesimalv1alpha1.EtcdNode{})
}
