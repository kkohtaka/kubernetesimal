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

package etcdnodedeployment

import (
	"context"
	"fmt"
	"math"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
)

// deploymentComplete considers a deployment to be complete once all of its desired replicas
// are updated and available, and no old pods are running.
func deploymentComplete(
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	newStatus *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus,
) bool {
	return newStatus.UpdatedReplicas == *(spec.Replicas) &&
		newStatus.Replicas == *(spec.Replicas) &&
		newStatus.AvailableReplicas == *(spec.Replicas) &&
		newStatus.ObservedGeneration >= obj.GetGeneration()
}

func cleanupDeployment(
	ctx context.Context,
	c client.Client,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	oldSets []*kubernetesimalv1alpha1.EtcdNodeSet,
) error {
	logger := log.FromContext(ctx)

	if !hasRevisionHistoryLimit(spec) {
		return nil
	}

	cleanableSets := filterAliveEtcdNodeSets(oldSets)

	diff := int32(len(cleanableSets)) - *spec.RevisionHistoryLimit
	if diff <= 0 {
		return nil
	}

	sort.Sort(etcdNodeSetsByRevision(cleanableSets))
	logger.V(4).Info("Looking to cleanup old EtcdNodeSets")

	for i := int32(0); i < diff; i++ {
		set := cleanableSets[i]
		// Avoid delete an EtcdNodeSet with non-zero replica counts
		if set.Status.Replicas != 0 ||
			*(set.Spec.Replicas) != 0 ||
			set.Generation > set.Status.ObservedGeneration ||
			set.DeletionTimestamp != nil {
			continue
		}
		logger.V(4).Info(
			"Trying to cleanup EtcdNodeSet",
			"etcdNodeSet", set.Name,
		)
		if err := c.Delete(ctx, set, &client.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			// Return error instead of aggregating and continuing DELETEs on the theory
			// that we may be overloading the api server.
			return fmt.Errorf("unable to delete an old EtcdNodeSet: %w", err)
		}
	}
	return nil
}

func hasRevisionHistoryLimit(spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec) bool {
	return spec.RevisionHistoryLimit != nil && *spec.RevisionHistoryLimit != math.MaxInt32
}

// syncRolloutStatus updates the status of a EtcdNodeDeployment during a rollout. There are cases this helper will run
// that cannot be prevented from the scaling detection, for example a resync of the EtcdNodeDeployment after it was
// scaled up. In those cases, we shouldn't try to estimate any progress.
func syncRolloutStatus(
	ctx context.Context,
	obj client.Object,
	status *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus,
	allSets []*kubernetesimalv1alpha1.EtcdNodeSet,
	newSet *kubernetesimalv1alpha1.EtcdNodeSet,
) *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus {
	availableReplicas := getAvailableReplicaCountForEtcdNodeSets(ctx, allSets)
	totalReplicas := getReplicaCountForEtcdNodeSets(allSets)
	unavailableReplicas := totalReplicas - availableReplicas
	// If unavailableReplicas is negative, then that means the Deployment has more available replicas running than
	// desired, e.g. whenever it scales down. In such a case we should simply default unavailableReplicas to zero.
	if unavailableReplicas < 0 {
		unavailableReplicas = 0
	}

	newStatus := &kubernetesimalv1alpha1.EtcdNodeDeploymentStatus{
		ObservedGeneration:  obj.GetGeneration(),
		Replicas:            getActualReplicaCountForEtcdNodeSets(allSets),
		UpdatedReplicas:     getActualReplicaCountForEtcdNodeSets([]*kubernetesimalv1alpha1.EtcdNodeSet{newSet}),
		ReadyReplicas:       getReadyReplicaCountForEtcdNodeSets(allSets),
		AvailableReplicas:   availableReplicas,
		UnavailableReplicas: unavailableReplicas,
		CollisionCount:      status.CollisionCount,
		Revision:            status.Revision,
	}
	return newStatus
}
