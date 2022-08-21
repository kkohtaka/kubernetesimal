/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
