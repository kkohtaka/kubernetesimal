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

	"sigs.k8s.io/controller-runtime/pkg/client"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
)

func syncStatus(
	ctx context.Context,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeSetSpec,
	status *kubernetesimalv1alpha1.EtcdNodeSetStatus,
	nodes []*kubernetesimalv1alpha1.EtcdNode,
) *kubernetesimalv1alpha1.EtcdNodeSetStatus {
	var (
		desiredReplicas   int32
		activeReplicas    int32
		availableReplicas int32
	)
	if spec.Replicas != nil {
		desiredReplicas = *spec.Replicas
	}
	for i := range nodes {
		activeReplicas++
		switch nodes[i].Status.Phase {
		case kubernetesimalv1alpha1.EtcdNodePhaseRunning:
			availableReplicas++
		default:
		}
	}

	newStatus := &kubernetesimalv1alpha1.EtcdNodeSetStatus{
		Replicas:           desiredReplicas,
		ActiveReplicas:     activeReplicas,
		ReadyReplicas:      availableReplicas,
		AvailableReplicas:  availableReplicas,
		ObservedGeneration: obj.GetGeneration(),
	}
	return newStatus
}
