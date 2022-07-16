package etcd

import (
	"context"
	"fmt"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/errors"
	"github.com/kkohtaka/kubernetesimal/controller/finalizer"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func removeOrphanNodes(
	ctx context.Context,
	c client.Client,
	directClient client.Reader,
	obj client.Object,
	status *kubernetesimalv1alpha1.EtcdStatus,
) error {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "removeOrphanNodes")
	defer span.End()

	logger := log.FromContext(ctx)

	childNodeNames := map[string]struct{}{}
	for _, nodeRef := range status.NodeRefs {
		childNodeNames[nodeRef.Name] = struct{}{}
	}

	var nodes kubernetesimalv1alpha1.EtcdNodeList
	if err := c.List(ctx, &nodes, &client.ListOptions{
		Namespace:     obj.GetNamespace(),
		LabelSelector: labels.Everything(),
	}); err != nil {
		return fmt.Errorf("unable to list EtcdNodes: %w", err)
	}

	for i := range nodes.Items {
		node := &nodes.Items[i]
		if _, ok := childNodeNames[node.GetName()]; ok {
			continue
		}
		for _, ref := range node.OwnerReferences {
			if ref.Controller == nil || !*ref.Controller {
				continue
			}
			if ref.UID != obj.GetUID() {
				continue
			}

			// Here, this EtcdNode is owned by the Etcd but not listed in Status.NodeRefs.
			if outdated, err := isStatusOutdated(ctx, directClient, obj, status); err != nil {
				return err
			} else if outdated {
				return errors.NewRequeueError("status is outdated")
			}

			logger.Info("Orphaned EtcdNode was found.", "etcdnode", node.GetName())
			if err := finalizeEtcdNode(ctx, c, obj.GetNamespace(), node.GetName()); err != nil {
				return fmt.Errorf("unable to finalize a EtcdNode: %w", err)
			}
		}
	}

	return nil
}

func finalizeEtcdNodes(
	ctx context.Context,
	c client.Client,
	e client.Object,
	status *kubernetesimalv1alpha1.EtcdStatus,
) (*kubernetesimalv1alpha1.EtcdStatus, error) {
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
	return finalizer.FinalizeObject(ctx, client, namespace, name, &kubernetesimalv1alpha1.EtcdNode{})
}
