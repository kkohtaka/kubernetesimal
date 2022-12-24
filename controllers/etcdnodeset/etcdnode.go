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

package etcdnodeset

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/kkohtaka/kubernetesimal/controller/expectations"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
	"go.opentelemetry.io/otel/trace"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	k8s_etcdnode "github.com/kkohtaka/kubernetesimal/k8s/etcdnode"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
)

func reconcileEtcdNodes(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	set client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeSetSpec,
	status *kubernetesimalv1alpha1.EtcdNodeSetStatus,
	expectations *expectations.UIDTrackingControllerExpectations,
) (*kubernetesimalv1alpha1.EtcdNodeSetStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileEtcdNodes")
	defer span.End()

	logger := log.FromContext(ctx)

	key := client.ObjectKeyFromObject(set).String()
	if needsSync := expectations.SatisfiedExpectations(key); !needsSync {
		return status, nil
	}

	activeNodes, err := getActiveEtcdNodes(ctx, c)
	if err != nil {
		return status, err
	}

	filteredNodes := filterControlledEtcdNodes(ctx, set, activeNodes)
	status.ActiveReplicas = int32(len(filteredNodes))

	diff := len(filteredNodes) - int(*spec.Replicas)
	if diff < 0 {
		diff *= -1
		if err := expectations.ExpectCreations(key, diff); err != nil {
			return nil, fmt.Errorf("unable to increment creation expectations: %w", err)
		}
		logger.V(2).Info("Too few replicas", "need", *(spec.Replicas), "creating", diff)

		templateSpec := &spec.Template.Spec

		var (
			wg     sync.WaitGroup
			errCh  = make(chan error, diff)
			nodeCh = make(chan *kubernetesimalv1alpha1.EtcdNode, diff)
		)
		wg.Add(diff)
		for i := 0; i < diff; i++ {
			go func() {
				defer wg.Done()

				if newNode, err := k8s_etcdnode.Create(
					ctx,
					c,
					k8s_object.WithGeneratorName(set.GetName()+"-"),
					k8s_object.WithNamespace(set.GetNamespace()),
					k8s_object.WithOwner(set, scheme),
					k8s_object.WithLabels(spec.Template.GetLabels()),
					k8s_etcdnode.WithVersion(templateSpec.Version),
					k8s_etcdnode.WithImagePersistentVolumeClaim(spec.Template.Spec.ImagePersistentVolumeClaimRef.Name),
					k8s_etcdnode.WithLoginPasswordSecretKeySelector(spec.Template.Spec.LoginPasswordSecretKeySelector),
					k8s_etcdnode.WithCACertificateRef(templateSpec.CACertificateRef),
					k8s_etcdnode.WithCAPrivateKeyRef(templateSpec.CAPrivateKeyRef),
					k8s_etcdnode.WithClientCertificateRef(templateSpec.ClientCertificateRef),
					k8s_etcdnode.WithClientPrivateKeyRef(templateSpec.ClientPrivateKeyRef),
					k8s_etcdnode.WithSSHPrivateKeyRef(templateSpec.SSHPrivateKeyRef),
					k8s_etcdnode.WithSSHPublicKeyRef(templateSpec.SSHPublicKeyRef),
					k8s_etcdnode.WithServiceRef(templateSpec.ServiceRef),
					k8s_etcdnode.AsFirstNode(templateSpec.AsFirstNode),
				); err != nil {
					errCh <- err
				} else {
					logger.Info("EtcdNode was created.", "node", newNode)
					nodeCh <- newNode
				}
			}()
		}
		wg.Wait()
		close(errCh)
		close(nodeCh)

		for node := range nodeCh {
			filteredNodes = append(filteredNodes, node)
		}
		var err error
		for e := range errCh {
			logger.Error(e, "Unable to create EtcdNode")
			expectations.CreationObserved(key)
			if err != nil {
				err = e
			}
		}
	} else if diff > 0 {
		logger.V(2).Info("Too many replicas", "need", *(spec.Replicas), "deleting", diff)

		nodesToDelete, err := getEtcdNodesToDelete(ctx, c, set, filteredNodes, filteredNodes, diff)
		if err != nil {
			return nil, fmt.Errorf("unable to get EtcdNodes to delete: %w", err)
		}

		if err := expectations.ExpectDeletions(key, getEtcdNodeKeys(nodesToDelete)); err != nil {
			return nil, fmt.Errorf("unable to increment deletion expectations: %w", err)
		}

		var (
			wg     sync.WaitGroup
			errCh  = make(chan error, diff)
			nodeCh = make(chan *kubernetesimalv1alpha1.EtcdNode, diff)
		)
		wg.Add(diff)
		for _, node := range nodesToDelete {
			go func(targetNode *kubernetesimalv1alpha1.EtcdNode) {
				defer wg.Done()
				if err := c.Delete(ctx, targetNode, &client.DeleteOptions{}); err != nil {
					nodeKey := client.ObjectKeyFromObject(targetNode).String()
					expectations.DeletionObserved(key, nodeKey)
					if !apierrors.IsNotFound(err) {
						logger.V(2).Info("Failed to delete", "etcdNode", nodeKey)
						errCh <- err
					}
				} else {
					nodeCh <- targetNode
				}
			}(node)
		}
		wg.Wait()
		close(errCh)
		close(nodeCh)

		for deletedNode := range nodeCh {
			for i, node := range filteredNodes {
				if node.UID == deletedNode.UID {
					filteredNodes = append(filteredNodes[:i], filteredNodes[i+1:]...)
					break
				}
			}
		}
		select {
		case err := <-errCh:
			if err != nil {
				return nil, err
			}
		default:
		}
	}
	return syncStatus(ctx, set, spec, status, filteredNodes), nil
}

func isActiveEtcdNode(node *kubernetesimalv1alpha1.EtcdNode) bool {
	return node.DeletionTimestamp == nil
}

func getActiveEtcdNodes(
	ctx context.Context,
	c client.Client,
) ([]*kubernetesimalv1alpha1.EtcdNode, error) {
	logger := log.FromContext(ctx)

	var nodeList kubernetesimalv1alpha1.EtcdNodeList
	if err := c.List(
		ctx,
		&nodeList,
		&client.ListOptions{
			// TODO(kkohtaka): Use labels
			LabelSelector: labels.Everything(),
		},
	); err != nil {
		return nil, fmt.Errorf("unable to list EtcdNodes: %w", err)
	}

	var nodes []*kubernetesimalv1alpha1.EtcdNode
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if isActiveEtcdNode(node) {
			nodes = append(nodes, node)
		} else {
			logger.V(4).Info("Ignoring inactive EtcdNode.",
				"etcdnode", fmt.Sprintf("%s/%s", node.Namespace, node.Name),
				"deletionTime", node.DeletionTimestamp,
			)
		}
	}
	return nodes, nil
}

func filterControlledEtcdNodes(
	_ context.Context,
	controller client.Object,
	nodes []*kubernetesimalv1alpha1.EtcdNode,
) []*kubernetesimalv1alpha1.EtcdNode {
	var filteredNodes []*kubernetesimalv1alpha1.EtcdNode
	for _, node := range nodes {
		if ref := metav1.GetControllerOf(node); ref == nil {
			continue
		} else if ref.UID != controller.GetUID() {
			continue
		}
		filteredNodes = append(filteredNodes, node)
	}
	return filteredNodes
}

type activeEtcdNodesWithRanks struct {
	EtcdNodes []*kubernetesimalv1alpha1.EtcdNode
	Rank      []int
	Now       metav1.Time
}

func (s activeEtcdNodesWithRanks) Len() int { return len(s.EtcdNodes) }
func (s activeEtcdNodesWithRanks) Swap(i, j int) {
	s.EtcdNodes[i], s.EtcdNodes[j] = s.EtcdNodes[j], s.EtcdNodes[i]
	s.Rank[i], s.Rank[j] = s.Rank[j], s.Rank[i]
}
func (s activeEtcdNodesWithRanks) Less(i, j int) bool {
	// Not ready < ready
	// If only one of the EtcdNodes is not ready, the not ready one is smaller
	if s.EtcdNodes[i].Status.IsReady() != s.EtcdNodes[j].Status.IsReady() {
		return !s.EtcdNodes[i].Status.IsReady()
	}

	// Doubled up < not doubled up
	// If one of the two EtcdNodes is on the same node as one or more additional
	// ready EtcdNodes that belong to the same EtcdNodeSet, whichever EtcdNode has more
	// co-located ready EtcdNode is less
	if s.Rank[i] != s.Rank[j] {
		return s.Rank[i] > s.Rank[j]
	}

	// Been ready for empty time < less time < more time
	// If both EtcdNodes are ready, the latest ready one is smaller
	if s.EtcdNodes[i].Status.IsReady() && s.EtcdNodes[j].Status.IsReady() {
		readyTime1 := s.EtcdNodes[i].Status.ReadySinceTime()
		readyTime2 := s.EtcdNodes[j].Status.ReadySinceTime()
		if !readyTime1.Equal(readyTime2) {
			if s.Now.IsZero() || readyTime1.IsZero() || readyTime2.IsZero() {
				return afterOrZero(readyTime1, readyTime2)
			}
			rankDiff := logarithmicRankDiff(*readyTime1, *readyTime2, s.Now)
			if rankDiff == 0 {
				return s.EtcdNodes[i].UID < s.EtcdNodes[j].UID
			}
			return rankDiff < 0
		}
	}

	// Empty creation time EtcdNodes < newer EtcdNodes < older EtcdNodes
	if !s.EtcdNodes[i].CreationTimestamp.Equal(&s.EtcdNodes[j].CreationTimestamp) {
		if s.Now.IsZero() || s.EtcdNodes[i].CreationTimestamp.IsZero() || s.EtcdNodes[j].CreationTimestamp.IsZero() {
			return afterOrZero(&s.EtcdNodes[i].CreationTimestamp, &s.EtcdNodes[j].CreationTimestamp)
		}
		rankDiff := logarithmicRankDiff(s.EtcdNodes[i].CreationTimestamp, s.EtcdNodes[j].CreationTimestamp, s.Now)
		if rankDiff == 0 {
			return s.EtcdNodes[i].UID < s.EtcdNodes[j].UID
		}
		return rankDiff < 0
	}
	return false
}

func afterOrZero(t1, t2 *metav1.Time) bool {
	if t1.Time.IsZero() || t2.Time.IsZero() {
		return t1.Time.IsZero()
	}
	return t1.After(t2.Time)
}

func logarithmicRankDiff(t1, t2, now metav1.Time) int64 {
	d1 := now.Sub(t1.Time)
	d2 := now.Sub(t2.Time)
	r1 := int64(-1)
	r2 := int64(-1)
	if d1 > 0 {
		r1 = int64(math.Log2(float64(d1)))
	}
	if d2 > 0 {
		r2 = int64(math.Log2(float64(d2)))
	}
	return r1 - r2
}

func getEtcdNodesToDelete(
	ctx context.Context,
	c client.Client,
	set client.Object,
	controlleeNodes, activeNodes []*kubernetesimalv1alpha1.EtcdNode,
	amount int,
) ([]*kubernetesimalv1alpha1.EtcdNode, error) {
	relatedNodes, err := getRelatedEtcdNodes(ctx, c, set, activeNodes)
	if err != nil {
		return nil, fmt.Errorf("unable to get related EtcdNodes: %w", err)
	}

	// # of EtcdNodes on a Node
	nodesOnNode := make(map[string]int)
	for i := range relatedNodes {
		node := relatedNodes[i]

		if !isActiveEtcdNode(node) {
			continue
		}

		if node.Status.VirtualMachineInstanceRef == nil {
			continue
		}

		var vmi kubevirtv1.VirtualMachineInstance
		if err := c.Get(
			ctx,
			client.ObjectKey{Namespace: node.Namespace, Name: node.Status.VirtualMachineInstanceRef.Name},
			&vmi,
		); err != nil {
			return nil, fmt.Errorf("unable to get VirtualMachineInstance: %w", err)
		}

		nodesOnNode[vmi.Status.NodeName]++
	}

	ranks := make([]int, len(controlleeNodes))
	for i := range controlleeNodes {
		node := relatedNodes[i]

		if node.Status.VirtualMachineInstanceRef == nil {
			continue
		}

		var vmi kubevirtv1.VirtualMachineInstance
		if err := c.Get(
			ctx,
			client.ObjectKey{Namespace: node.Namespace, Name: node.Status.VirtualMachineInstanceRef.Name},
			&vmi,
		); err != nil {
			return nil, fmt.Errorf("unable to get VirtualMachineInstance: %w", err)
		}

		ranks[i] = nodesOnNode[vmi.Status.NodeName]
	}

	sortable := activeEtcdNodesWithRanks{
		EtcdNodes: controlleeNodes,
		Rank:      ranks,
		Now:       metav1.Now(),
	}
	sort.Sort(sortable)

	if amount > len(sortable.EtcdNodes) {
		amount = len(sortable.EtcdNodes)
	}
	return sortable.EtcdNodes[:amount], nil
}

func getRelatedEtcdNodes(
	ctx context.Context,
	c client.Client,
	set client.Object,
	activeNodes []*kubernetesimalv1alpha1.EtcdNode,
) ([]*kubernetesimalv1alpha1.EtcdNode, error) {
	var ownerUID types.UID
	if ref := metav1.GetControllerOf(set); ref != nil {
		ownerUID = ref.UID
	}

	relatedNodeSetUIDs := make(map[types.UID]struct{})
	relatedNodeSetUIDs[set.GetUID()] = struct{}{}

	if len(ownerUID) > 0 {
		var nodeSetList kubernetesimalv1alpha1.EtcdNodeSetList
		if err := c.List(
			ctx,
			&nodeSetList,
			&client.ListOptions{
				LabelSelector: labels.Everything(),
			},
		); err != nil {
			return nil, fmt.Errorf("unable to list EtcdNodeSets: %w", err)
		}

		for i := range nodeSetList.Items {
			nodeSet := nodeSetList.Items[i]
			if nodeSet.UID == set.GetUID() {
				continue
			}

			var uid types.UID
			for _, ref := range nodeSet.OwnerReferences {
				if ref.Controller == nil || !*ref.Controller {
					continue
				}
				uid = ref.UID
				break
			}
			if uid != ownerUID {
				continue
			}

			relatedNodeSetUIDs[nodeSet.UID] = struct{}{}
		}
	}

	var relatedNodes []*kubernetesimalv1alpha1.EtcdNode
	for i := range activeNodes {
		node := activeNodes[i]
		for _, ref := range node.OwnerReferences {
			if ref.Controller == nil || !*ref.Controller {
				continue
			}

			if _, ok := relatedNodeSetUIDs[ref.UID]; ok {
				relatedNodes = append(relatedNodes, node)
			}
			break
		}
	}

	return relatedNodes, nil
}

func getEtcdNodeKeys(objs []*kubernetesimalv1alpha1.EtcdNode) []string {
	keys := make([]string, 0, len(objs))
	for _, obj := range objs {
		keys = append(keys, client.ObjectKeyFromObject(obj).String())
	}
	return keys
}
