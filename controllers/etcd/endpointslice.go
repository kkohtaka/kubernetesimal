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

package etcd

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	pointerutils "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/errors"
	k8s_endpointslice "github.com/kkohtaka/kubernetesimal/k8s/endpointslice"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

func newEndpointSliceName(e client.Object) string {
	return e.GetName()
}

func reconcileEndpointSlice(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	obj client.Object,
	_ *kubernetesimalv1alpha1.EtcdSpec,
	status *kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.LocalObjectReference, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileEndpointSlice")
	defer span.End()
	logger := log.FromContext(ctx)

	var service corev1.Service
	if err := c.Get(
		ctx,
		types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      status.ServiceRef.Name,
		},
		&service,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewRequeueError("waiting for the etcd Service prepared").Wrap(err)
		}
		return nil, err
	}

	nodes, err := getComponentEtcdNodes(ctx, c, obj)
	if err != nil {
		return nil, fmt.Errorf("unable to list component EtcdNodes: %w", err)
	}

	var endpoints []discoveryv1.Endpoint
	for _, node := range nodes {
		nodeKey := client.ObjectKeyFromObject(node)

		if node.Status.PeerServiceRef == nil {
			logger.
				WithValues("etcd-node", nodeKey).
				Info("Skip appending an endpoint since EtcdNode doesn't have a Service for peer communications.")
			continue
		}
		var (
			peerService    corev1.Service
			peerServiceKey = types.NamespacedName{
				Namespace: node.Namespace,
				Name:      node.Status.PeerServiceRef.Name,
			}
		)
		if err := c.Get(
			ctx,
			peerServiceKey,
			&peerService,
		); err != nil {
			if apierrors.IsNotFound(err) {
				logger.
					WithValues("etcd-node", nodeKey).
					WithValues("service", peerServiceKey).
					Info("Skip appending an endpoint since Service is not found.")
				continue
			}
			return nil, err
		}
		if len(peerService.Spec.ClusterIPs) == 0 {
			logger.
				WithValues("etcd-node", nodeKey).
				WithValues("service", peerServiceKey).
				Info("Skip appending an endpoint since a Service doesn't have a cluster IP.")
			continue
		}

		var (
			serving     = node.Status.IsReady()
			terminating = !node.DeletionTimestamp.IsZero() || !peerService.DeletionTimestamp.IsZero()
			ready       = serving && !terminating
		)

		endpoints = append(endpoints, discoveryv1.Endpoint{
			Addresses: peerService.Spec.ClusterIPs,
			Hostname:  pointerutils.StringPtr(peerService.Name),
			Conditions: discoveryv1.EndpointConditions{
				Ready:       &ready,
				Serving:     &serving,
				Terminating: &terminating,
			},
			TargetRef: &corev1.ObjectReference{
				Kind:       peerService.Kind,
				Namespace:  peerService.Namespace,
				Name:       peerService.Name,
				UID:        peerService.UID,
				APIVersion: peerService.APIVersion,
			},
		})
	}

	if ep, err := k8s_endpointslice.Reconcile(
		ctx,
		obj,
		c,
		newEndpointSliceName(obj),
		obj.GetNamespace(),
		k8s_object.WithOwner(obj, scheme),
		k8s_object.WithLabel("kubernetes.io/service-name", service.Name),
		k8s_object.WithLabel("endpointslice.kubernetes.io/managed-by", "etcd-controller.kubernetesimal.kkohtaka.org"),
		k8s_endpointslice.WithAddressType(discoveryv1.AddressTypeIPv4),
		k8s_endpointslice.WithPort(ServiceNameEtcd, ServicePortEtcd),
		k8s_endpointslice.WithEndpoints(endpoints),
	); err != nil {
		return nil, fmt.Errorf("unable to prepare an EndpointSlice for an etcd cluster: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: ep.Name,
		}, nil
	}
}
