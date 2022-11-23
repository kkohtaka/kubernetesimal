package etcd

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
)

func getComponentEtcdNodes(ctx context.Context, c client.Client, e client.Object) ([]*kubernetesimalv1alpha1.EtcdNode, error) {
	var nodeList kubernetesimalv1alpha1.EtcdNodeList
	if err := c.List(
		ctx,
		&nodeList,
		&client.ListOptions{
			LabelSelector: labels.SelectorFromSet(
				newEtcdNodeTemplateSpecLabels(e),
			),
		},
	); err != nil {
		return nil, fmt.Errorf("unable to list EtcdNodes: %w", err)
	}

	var nodes []*kubernetesimalv1alpha1.EtcdNode
	for i := range nodeList.Items {
		nodes = append(nodes, &nodeList.Items[i])
	}
	return nodes, nil
}
