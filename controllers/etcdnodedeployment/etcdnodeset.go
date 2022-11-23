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

package etcdnodedeployment

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/integer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/hash"
	k8s_etcdnodeset "github.com/kkohtaka/kubernetesimal/k8s/etcdnodeset"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

func reconcileEtcdNodeSets(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	deployment client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	status *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus,
) (*kubernetesimalv1alpha1.EtcdNodeDeploymentStatus, error) {
	logger := log.FromContext(ctx)

	ctx, span := tracing.FromContext(ctx).Start(ctx, "reconcileEtcdNodeSets")
	defer span.End()

	sets, err := getEtcdNodeSetsForEtcdNodeDeployment(ctx, c, deployment)
	if err != nil {
		return nil, err
	}

	newSet, oldSets, newRevision, collision, err := getAllEtcdNodeSetsAndSyncRevision(
		ctx,
		c,
		deployment,
		spec,
		sets,
		status,
	)
	if err != nil {
		return nil, err
	} else if collision {
		if status.CollisionCount == nil {
			status.CollisionCount = new(int32)
		}
		*status.CollisionCount++
		logger.V(2).Info("Found a hash collision", "collisionCount", *status.CollisionCount)
		return status, nil
	} else {
		status.Revision = &newRevision
	}

	allSets := append(oldSets, newSet)

	// Scale up, if we can.
	scaledUp, err := reconcileNewEtcdNodeSet(ctx, c, spec, allSets, newSet)
	if err != nil {
		return nil, err
	}
	if scaledUp {
		return syncRolloutStatus(ctx, deployment, status, allSets, newSet), nil
	}

	// Scale down, if we can.
	scaledDown, err := reconcileOldEtcdNodeSets(ctx, c, spec, allSets, filterActiveEtcdNodeSets(oldSets), newSet)
	if err != nil {
		return nil, err
	}
	if scaledDown {
		return syncRolloutStatus(ctx, deployment, status, allSets, newSet), nil
	}

	if deploymentComplete(deployment, spec, status) {
		if err := cleanupDeployment(ctx, c, spec, oldSets); err != nil {
			return nil, err
		}
	}
	return syncRolloutStatus(ctx, deployment, status, allSets, newSet), nil
}

func getEtcdNodeSetsForEtcdNodeDeployment(
	ctx context.Context,
	c client.Client,
	obj client.Object,
) ([]*kubernetesimalv1alpha1.EtcdNodeSet, error) {
	var setList kubernetesimalv1alpha1.EtcdNodeSetList
	if err := c.List(ctx, &setList, &client.ListOptions{LabelSelector: labels.Everything()}); err != nil {
		return nil, fmt.Errorf("unable to list EtcdNodeSets: %w", err)
	}

	var sets []*kubernetesimalv1alpha1.EtcdNodeSet
	for i := range setList.Items {
		set := &setList.Items[i]
		if ref := metav1.GetControllerOf(set); ref == nil {
			continue
		} else if ref.UID != obj.GetUID() {
			continue
		}
		sets = append(sets, set)
	}
	return sets, nil
}

func getAllEtcdNodeSetsAndSyncRevision(
	ctx context.Context,
	c client.Client,
	deployment client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	sets []*kubernetesimalv1alpha1.EtcdNodeSet,
	status *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus,
) (
	*kubernetesimalv1alpha1.EtcdNodeSet,
	[]*kubernetesimalv1alpha1.EtcdNodeSet,
	int64,
	bool,
	error,
) {
	_, allOldSets := findOldEtcdNodeSets(spec, sets)

	// Get new EtcdNodeSet with the updated revision number
	if newSet, newRevision, collision, err := getNewEtcdNodeSet(
		ctx,
		c,
		deployment,
		spec,
		sets,
		allOldSets,
		status,
	); err != nil {
		return nil, nil, 0, false, err
	} else {
		return newSet, allOldSets, newRevision, collision, nil
	}
}

func reconcileNewEtcdNodeSet(
	ctx context.Context,
	c client.Client,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	allSets []*kubernetesimalv1alpha1.EtcdNodeSet,
	newSet *kubernetesimalv1alpha1.EtcdNodeSet,
) (bool, error) {
	ctx, span := tracing.FromContext(ctx).Start(ctx, "reconcileNewEtcdNodeSet")
	defer span.End()

	if *(newSet.Spec.Replicas) == *(spec.Replicas) {
		return false, nil
	}
	if *(newSet.Spec.Replicas) > *(spec.Replicas) {
		scaled, _, err := scaleEtcdNodeSet(ctx, c, spec, newSet, *(spec.Replicas))
		return scaled, err
	}
	newReplicasCount, err := newEtcdNodeSetNewReplicas(spec, allSets, *newSet.Spec.Replicas)
	if err != nil {
		return false, err
	}
	scaled, _, err := scaleEtcdNodeSet(ctx, c, spec, newSet, newReplicasCount)
	return scaled, err
}

func reconcileOldEtcdNodeSets(
	ctx context.Context,
	c client.Client,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	allSets, oldSets []*kubernetesimalv1alpha1.EtcdNodeSet,
	newSet *kubernetesimalv1alpha1.EtcdNodeSet,
) (bool, error) {
	logger := log.FromContext(ctx)

	ctx, span := tracing.FromContext(ctx).Start(ctx, "reconcileOldEtcdNodeSets")
	defer span.End()

	oldNodesCount := getEtcdNodeCountForEtcdNodeSets(oldSets)
	if oldNodesCount == 0 {
		// Can't scale down further
		return false, nil
	}

	allNodesCount := getEtcdNodeCountForEtcdNodeSets(allSets)
	logger.V(4).Info(
		"New EtcdNodeSet has available pods.",
		"etcdNodeSet", client.ObjectKeyFromObject(newSet).String(),
		"availableReplicas", newSet.Status.AvailableReplicas,
	)
	maxUnavailable := maxUnavailable(spec)

	minAvailable := *(spec.Replicas) - maxUnavailable
	newSetUnavailableNodesCount := *(newSet.Spec.Replicas) - newSet.Status.AvailableReplicas
	maxScaledDown := allNodesCount - minAvailable - newSetUnavailableNodesCount
	if maxScaledDown <= 0 {
		return false, nil
	}

	oldSets, cleanupCount, err := cleanupUnhealthyReplicas(ctx, c, spec, oldSets, maxScaledDown)
	if err != nil {
		return false, nil
	}
	logger.V(4).Info(
		"Unhealthy replicas are cleaned up.",
		"cleanupCount", cleanupCount,
	)

	// Scale down old EtcdNodeSets, need check maxUnavailable to ensure we can scale down
	allSets = append(oldSets, newSet)
	scaledDownCount, err := scaleDownOldReplicaSetsForRollingUpdate(ctx, c, spec, allSets, oldSets)
	if err != nil {
		return false, nil
	}
	logger.V(4).Info(
		"Old EtcdNodeSets are scaled down.",
		"scaleDownCount", scaledDownCount,
	)

	totalScaledDown := cleanupCount + scaledDownCount
	return totalScaledDown > 0, nil
}

func findOldEtcdNodeSets(
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	sets []*kubernetesimalv1alpha1.EtcdNodeSet,
) (requiredSets, allSets []*kubernetesimalv1alpha1.EtcdNodeSet) {
	newSet := findNewEtcdNodeSet(spec, sets)
	for _, set := range sets {
		if newSet != nil && set.GetUID() == newSet.UID {
			continue
		}
		allSets = append(allSets, set)
		if *(set.Spec.Replicas) == 0 {
			continue
		}
		requiredSets = append(requiredSets, set)
	}
	return requiredSets, allSets
}

type etcdNodeSetsByCreationTimestamp []*kubernetesimalv1alpha1.EtcdNodeSet

func (o etcdNodeSetsByCreationTimestamp) Len() int      { return len(o) }
func (o etcdNodeSetsByCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o etcdNodeSetsByCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}

func findNewEtcdNodeSet(
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	sets []*kubernetesimalv1alpha1.EtcdNodeSet,
) *kubernetesimalv1alpha1.EtcdNodeSet {
	sort.Sort(etcdNodeSetsByCreationTimestamp(sets))
	for i := range sets {
		if equalIgnoreHash(&sets[i].Spec.Template, &spec.Template) {
			return sets[i]
		}
	}
	return nil
}

const (
	// limit revision history length to 100 element (~2000 chars)
	maxRevHistoryLengthInChars = 2000

	// defaultDeploymentUniqueLabelKey is the default key of the selector that is added to existing EtcdNodeSets (and
	// label key that is added to its EtcdNode) to prevent the existing EtcdNodeSets to select new EtcdNodes (and old
	// EtcdNodes being select by new EtcdNodeSet).
	defaultDeploymentUniqueLabelKey = "etcd-node-template-hash"
)

// Returns an EtcdNodeSet that matches the intent of the given deployment. Returns nil if the new EtcdNodeSet doesn't
// exist yet.
func getNewEtcdNodeSet(
	ctx context.Context,
	c client.Client,
	deployment client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	sets, oldSets []*kubernetesimalv1alpha1.EtcdNodeSet,
	status *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus,
) (
	*kubernetesimalv1alpha1.EtcdNodeSet,
	int64,
	bool,
	error,
) {
	existingNewSet := findNewEtcdNodeSet(spec, sets)

	// Calculate the max revision number among all old Sets
	maxOldRevision := maxRevision(ctx, oldSets)
	newRevision := maxOldRevision + 1
	// Calculate revision number for this new EtcdNodeSet
	newRevisionString := strconv.FormatInt(newRevision, 10)
	var oldRevisionString string
	if status.Revision != nil {
		oldRevisionString = strconv.FormatInt(*status.Revision, 10)
	}

	if existingNewSet != nil {
		newAnnotations := newEtcdNodeSetAnnotations(
			ctx,
			deployment,
			existingNewSet.Annotations,
			spec,
			newRevisionString,
			oldRevisionString,
			true,
			maxRevHistoryLengthInChars,
		)
		op, updatedSet, err := k8s_etcdnodeset.Reconcile(
			ctx,
			c,
			existingNewSet.Name,
			existingNewSet.Namespace,
			k8s_object.WithAnnotations(newAnnotations),
		)
		if err != nil {
			return nil, 0, false, fmt.Errorf("unable to update EtcdNodeSet's annotations: %w", err)
		}
		if op != controllerutil.OperationResultNone {
			return updatedSet, newRevision, false, nil
		}

		newRevision, err = revision(existingNewSet)
		if err != nil {
			return nil, 0, false, fmt.Errorf("unable to get revision from EtcdNodeSet: %w", err)
		}
		return existingNewSet, newRevision, false, nil
	}

	// new EtcdNodeSet does not exist, create one.
	newSetTemplate := *spec.Template.DeepCopy()
	nodeTemplateSpecHash := computeHash(&newSetTemplate, status.CollisionCount)
	newSetTemplate.Labels = cloneAndAddLabel(
		spec.Template.Labels,
		defaultDeploymentUniqueLabelKey,
		nodeTemplateSpecHash,
	)
	// Add podTemplateHash label to selector.
	newSetSelector := cloneSelectorAndAddLabel(spec.Selector, defaultDeploymentUniqueLabelKey, nodeTemplateSpecHash)

	newSetReplicas, err := newEtcdNodeSetNewReplicas(spec, oldSets, 0)
	if err != nil {
		return nil, 0, false, fmt.Errorf("unable to calculate a number of replicas for a new EtcdNodeSet: %w", err)
	}

	newSetAnnotations := make(map[string]string)
	newEtcdNodeSetAnnotations(
		ctx,
		deployment,
		newSetAnnotations,
		spec,
		newRevisionString,
		oldRevisionString,
		false,
		maxRevHistoryLengthInChars,
	)

	// Create the new EtcdNodeSet.  If it already exists, then we need to check for possible hash collisions.  If there
	// is any other error, we need to report it in the status of the EtcdNodeDeployment.
	opRes, newSet, err := k8s_etcdnodeset.CreateOnlyIfNotExist(
		ctx,
		c,
		deployment.GetName()+"-"+nodeTemplateSpecHash,
		deployment.GetNamespace(),
		k8s_object.WithAnnotations(newSetAnnotations),
		k8s_object.WithLabels(newSetTemplate.GetLabels()),
		k8s_object.WithOwner(deployment, c.Scheme()),
		k8s_etcdnodeset.WithReplicas(newSetReplicas),
		k8s_etcdnodeset.WithTemplate(newSetTemplate),
		k8s_etcdnodeset.WithSelector(newSetSelector),
	)
	if err != nil {
		return nil, 0, false, fmt.Errorf("unable to create EtcdNodeSet: %w", err)
	}

	switch opRes {
	// We may end up hitting this due to a slow cache or a fast resync of the EtcdNodeDeployment.
	case controllerutil.OperationResultNone:
		// If the EtcdNodeDeployment owns the EtcdNodeSet and the EtcdNodeSet's EtcdNodeTemplateSpec is semantically
		// deep equal to the EtcdNodeTemplateSpec of the EtcdNodeDeployment, it's the EtcdNodeDeployment's new
		// EtcdNodeSet.
		// Otherwise, this is a hash collision and we need to increment the collisionCount field in the status of the
		// EtcdNodeDeployment and requeue to try the creation in the next sync.
		controllerRef := metav1.GetControllerOf(newSet)
		if controllerRef != nil && controllerRef.UID == deployment.GetUID() &&
			equalIgnoreHash(&spec.Template, &newSet.Spec.Template) {
			break
		}
		return newSet, newRevision, true, nil
	}

	newRevision, err = revision(newSet)
	if err != nil {
		return nil, 0, false, fmt.Errorf("unable to get revision from EtcdNodeSet: %w", err)
	}
	return newSet, newRevision, false, nil
}

// computeHash returns a hash value calculated from EtcdNodeTemplateSpec and a collisionCount to avoid hash collision.
// The hash will be safe encoded to avoid bad words.
func computeHash(template *kubernetesimalv1alpha1.EtcdNodeTemplateSpec, collisionCount *int32) string {
	templateSpecHasher := fnv.New32a()
	hash.DeepHashObject(templateSpecHasher, *template)

	// Add collisionCount in the hash if it exists.
	if collisionCount != nil {
		collisionCountBytes := make([]byte, 8)
		binary.LittleEndian.PutUint32(collisionCountBytes, uint32(*collisionCount))
		templateSpecHasher.Write(collisionCountBytes)
	}
	return rand.SafeEncodeString(fmt.Sprint(templateSpecHasher.Sum32()))
}

// equalIgnoreHash returns true if two given EtcdNodeTemplateSpec are equal, ignoring the diff in value of
// Labels[etcd-node-template-hash]
func equalIgnoreHash(template1, template2 *kubernetesimalv1alpha1.EtcdNodeTemplateSpec) bool {
	t1Copy := template1.DeepCopy()
	t2Copy := template2.DeepCopy()
	// Remove hash labels from template.Labels before comparing
	delete(t1Copy.Labels, defaultDeploymentUniqueLabelKey)
	delete(t2Copy.Labels, defaultDeploymentUniqueLabelKey)
	return apiequality.Semantic.DeepEqual(t1Copy, t2Copy)
}

func cloneAndAddLabel(
	labels map[string]string,
	labelKey, labelValue string,
) map[string]string {
	if labelKey == "" {
		// Don't need to add a label.
		return labels
	}
	// Clone.
	newLabels := map[string]string{}
	for key, value := range labels {
		newLabels[key] = value
	}
	newLabels[labelKey] = labelValue
	return newLabels
}

func cloneSelectorAndAddLabel(
	selector *metav1.LabelSelector,
	labelKey, labelValue string,
) *metav1.LabelSelector {
	if labelKey == "" {
		// Don't need to add a label.
		return selector
	}

	newSelector := new(metav1.LabelSelector)
	newSelector.MatchLabels = make(map[string]string)
	if selector != nil && selector.MatchLabels != nil {
		for key, val := range selector.MatchLabels {
			newSelector.MatchLabels[key] = val
		}
	}
	newSelector.MatchLabels[labelKey] = labelValue

	if selector != nil && selector.MatchExpressions != nil {
		newMExps := make([]metav1.LabelSelectorRequirement, len(selector.MatchExpressions))
		for i, me := range selector.MatchExpressions {
			newMExps[i].Key = me.Key
			newMExps[i].Operator = me.Operator
			if me.Values != nil {
				newMExps[i].Values = make([]string, len(me.Values))
				copy(newMExps[i].Values, me.Values)
			} else {
				newMExps[i].Values = nil
			}
		}
		newSelector.MatchExpressions = newMExps
	} else {
		newSelector.MatchExpressions = nil
	}

	return newSelector
}

func maxRevision(ctx context.Context, allSets []*kubernetesimalv1alpha1.EtcdNodeSet) int64 {
	logger := log.FromContext(ctx)

	max := int64(0)
	for _, set := range allSets {
		if v, err := revision(set); err != nil {
			// Skip the EtcdNodeSets when it failed to parse their revision information
			logger.V(4).Error(err,
				"Couldn't parse a revision for an EtcdNodeSet. An EtcdNodeDeployment controller will skip it when reconciling revisions.",
				"etcd-node-set", set,
			)
		} else if v > max {
			max = v
		}
	}
	return max
}

const (
	// RevisionAnnotation is the revision annotation of a deployment's EtcdNodeSets which records its rollout sequence
	RevisionAnnotation = "etcdnodedeployment.kubernetesimal.kkohtaka.org/revision"
	// RevisionHistoryAnnotation maintains the history of all old revisions that an EtcdNodeSet has served for a deployment.
	RevisionHistoryAnnotation = "etcdnodedeployment.kubernetesimal.kkohtaka.org/revision-history"
	// DesiredReplicasAnnotation is the desired replicas for a deployment recorded as an annotation
	// in its EtcdNodeSets. Helps in separating scaling events from the rollout process and for
	// determining if the new EtcdNodeSet for a deployment is really saturated.
	DesiredReplicasAnnotation = "etcdnodedeployment.kubernetesimal.kkohtaka.org/desired-replicas"
	// MaxReplicasAnnotation is the maximum replicas a deployment can have at a given point, which
	// is deployment.spec.replicas + maxSurge. Used by the underlying EtcdNodeSets to estimate their
	// proportions in case the deployment has surge replicas.
	MaxReplicasAnnotation = "etcdnodedeployment.kubernetesimal.kkohtaka.org/max-replicas"
)

// revision returns the revision number of the input object.
func revision(set *kubernetesimalv1alpha1.EtcdNodeSet) (int64, error) {
	acc, err := meta.Accessor(set)
	if err != nil {
		return 0, err
	}
	v, ok := acc.GetAnnotations()[RevisionAnnotation]
	if !ok {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}

// newEtcdNodeSetAnnotations returns new EtcdNodeSet's annotations appropriately by updating its revision and copying
// required EtcdNodeSetDeployment annotations to it.
func newEtcdNodeSetAnnotations(
	ctx context.Context,
	deployment client.Object,
	annotations map[string]string,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	newRevision string,
	oldRevision string,
	exists bool,
	revHistoryLimitInChars int,
) map[string]string {
	logger := log.FromContext(ctx)

	annotations = copyAnnotationsToEtcdNodeSet(deployment, annotations)
	if annotations == nil {
		annotations = make(map[string]string)
	}

	oldRevisionInt, err := strconv.ParseInt(oldRevision, 10, 64)
	if err != nil {
		if oldRevision != "" {
			logger.Error(
				err,
				"Unable to update a revision of an EtcdNodeSet, since the old revision is not an integer",
				"old-revision", oldRevision,
			)
			return annotations
		}
		oldRevisionInt = 0
	}
	newRevisionInt, err := strconv.ParseInt(newRevision, 10, 64)
	if err != nil {
		logger.Error(
			err,
			"Unable to update a revision of an EtcdNodeSet, since the new revision is not an integer",
			"new-revision", newRevision,
		)
		return annotations
	}
	if oldRevisionInt < newRevisionInt {
		annotations[RevisionAnnotation] = newRevision
		logger.V(4).Info(
			"Updating a revision of an EtcdNodeSet",
			"revision", newRevision,
		)
	}

	// If a revision annotation already existed and this EtcdNodeSet was updated with a new revision then that means we
	// are rolling back to this EtcdNodeSet.  We need to preserve the old revisions for historical information.
	if oldRevisionInt < newRevisionInt {
		revisionHistoryAnnotation := annotations[RevisionHistoryAnnotation]
		oldRevisions := strings.Split(revisionHistoryAnnotation, ",")
		if len(oldRevisions[0]) == 0 {
			annotations[RevisionHistoryAnnotation] = oldRevision
		} else {
			totalLen := len(revisionHistoryAnnotation) + len(oldRevision) + 1
			// index for the starting position in oldRevisions
			start := 0
			for totalLen > revHistoryLimitInChars && start < len(oldRevisions) {
				totalLen = totalLen - len(oldRevisions[start]) - 1
				start++
			}
			if totalLen <= revHistoryLimitInChars {
				oldRevisions = append(oldRevisions[start:], oldRevision)
				annotations[RevisionHistoryAnnotation] = strings.Join(oldRevisions, ",")
			} else {
				logger.Info("Not appending revision due to length limit of %v reached", revHistoryLimitInChars)
			}
		}
	}
	// If the new EtcdNodeSet is about to be created, we need to add replica annotations to it.
	if !exists {
		annotations = setReplicasAnnotations(annotations, *(spec.Replicas), *(spec.Replicas)+maxSurge(spec))
	}
	return annotations
}

// copyAnnotationsToEtcdNodeSet copies EtcdNodeDeployment's annotations to EtcdNodeSet's, and returns them.  Note that
// apply and revision annotations are not copied.
func copyAnnotationsToEtcdNodeSet(
	obj client.Object,
	annotations map[string]string,
) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	for k, v := range obj.GetAnnotations() {
		if _, exist := annotations[k]; skipCopyAnnotation(k) || (exist && annotations[k] == v) {
			continue
		}
		annotations[k] = v
	}
	return annotations
}

var annotationsToSkip = map[string]bool{
	RevisionAnnotation:        true,
	RevisionHistoryAnnotation: true,
	DesiredReplicasAnnotation: true,
	MaxReplicasAnnotation:     true,
}

func skipCopyAnnotation(key string) bool {
	return annotationsToSkip[key]
}

// setReplicasAnnotations sets the desiredReplicas and maxReplicas into the annotations
func setReplicasAnnotations(
	annotations map[string]string,
	desiredReplicas, maxReplicas int32,
) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	desiredString := fmt.Sprintf("%d", desiredReplicas)
	if hasString := annotations[DesiredReplicasAnnotation]; hasString != desiredString {
		annotations[DesiredReplicasAnnotation] = desiredString
	}
	maxString := fmt.Sprintf("%d", maxReplicas)
	if hasString := annotations[MaxReplicasAnnotation]; hasString != maxString {
		annotations[MaxReplicasAnnotation] = maxString
	}
	return annotations
}

func withDesiredReplicasAnnotation(desiredReplicas int32) k8s_object.ObjectOption {
	return k8s_object.WithAnnotation(DesiredReplicasAnnotation, fmt.Sprintf("%d", desiredReplicas))
}

func withMaxReplicasAnnotation(maxReplicas int32) k8s_object.ObjectOption {
	return k8s_object.WithAnnotation(MaxReplicasAnnotation, fmt.Sprintf("%d", maxReplicas))
}

// maxSurge returns the maximum surge pods a rolling deployment can take.
func maxSurge(spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec) int32 {
	// Error caught by validation
	maxSurge, _, _ := resolveFenceposts(
		spec.RollingUpdate.MaxSurge,
		spec.RollingUpdate.MaxUnavailable,
		*(spec.Replicas),
	)
	return maxSurge
}

// resolveFenceposts resolves both maxSurge and maxUnavailable. This needs to happen in one step. For example:
//
// 2 desired, max unavailable 1%, surge 0% - should scale old(-1), then new(+1), then old(-1), then new(+1)
// 1 desired, max unavailable 1%, surge 0% - should scale old(-1), then new(+1)
// 2 desired, max unavailable 25%, surge 1% - should scale new(+1), then old(-1), then new(+1), then old(-1)
// 1 desired, max unavailable 25%, surge 1% - should scale new(+1), then old(-1)
// 2 desired, max unavailable 0%, surge 1% - should scale new(+1), then old(-1), then new(+1), then old(-1)
// 1 desired, max unavailable 0%, surge 1% - should scale new(+1), then old(-1)
func resolveFenceposts(maxSurge, maxUnavailable *intstrutil.IntOrString, desired int32) (int32, int32, error) {
	surge, err := intstrutil.GetScaledValueFromIntOrPercent(
		intstrutil.ValueOrDefault(maxSurge, intstrutil.FromInt(0)),
		int(desired),
		true,
	)
	if err != nil {
		return 0, 0, err
	}
	unavailable, err := intstrutil.GetScaledValueFromIntOrPercent(
		intstrutil.ValueOrDefault(maxUnavailable, intstrutil.FromInt(0)),
		int(desired),
		false,
	)
	if err != nil {
		return 0, 0, err
	}

	if surge == 0 && unavailable == 0 {
		// Validation should never allow the user to explicitly use zero values for both maxSurge
		// maxUnavailable. Due to rounding down maxUnavailable though, it may resolve to zero.
		// If both fenceposts resolve to zero, then we should set maxUnavailable to 1 on the
		// theory that surge might not work due to quota.
		unavailable = 1
	}

	return int32(surge), int32(unavailable), nil
}

func newEtcdNodeSetNewReplicas(
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	allSets []*kubernetesimalv1alpha1.EtcdNodeSet,
	currentSpecReplicas int32,
) (int32, error) {
	// Check if we can scale up.
	maxSurge, err := intstrutil.GetScaledValueFromIntOrPercent(
		spec.RollingUpdate.MaxSurge,
		int(*(spec.Replicas)),
		true,
	)
	if err != nil {
		return 0, err
	}

	// Find the total number of nodes
	currentNodeCount := getReplicaCountForEtcdNodeSets(allSets)
	maxTotalNodes := *(spec.Replicas) + int32(maxSurge)
	if currentNodeCount >= maxTotalNodes {
		// Cannot scale up.
		return currentSpecReplicas, nil
	}

	// Scale up.
	scaleUpCount := maxTotalNodes - currentNodeCount

	// Do not exceed the number of desired replicas.
	scaleUpCount = int32(integer.IntMin(int(scaleUpCount), int(*(spec.Replicas)-currentSpecReplicas)))
	return currentSpecReplicas + scaleUpCount, nil
}

// getReplicaCountForEtcdNodeSets returns the sum of Replicas of the given EtcdNodeSets.
func getReplicaCountForEtcdNodeSets(replicaSets []*kubernetesimalv1alpha1.EtcdNodeSet) int32 {
	totalReplicas := int32(0)
	for _, set := range replicaSets {
		if set != nil {
			totalReplicas += *(set.Spec.Replicas)
		}
	}
	return totalReplicas
}

// getAvailableReplicaCountForEtcdNodeSets returns the number of available EtcdNodes corresponding to the given
// EtcdNodeSets.
func getAvailableReplicaCountForEtcdNodeSets(
	ctx context.Context,
	sets []*kubernetesimalv1alpha1.EtcdNodeSet,
) int32 {
	logger := log.FromContext(ctx)
	totalAvailableReplicas := int32(0)
	for _, set := range sets {
		if set != nil && set.Status.AvailableReplicas > 0 {
			logger.Info(
				"Available EtcdNodes are found in EtcdNodeSet",
				"count", set.Status.AvailableReplicas,
				"set", client.ObjectKeyFromObject(set),
			)
			totalAvailableReplicas += set.Status.AvailableReplicas
		}
	}
	return totalAvailableReplicas
}

// getActualReplicaCountForEtcdNodeSets returns the sum of actual replicas of the given replica sets.
func getActualReplicaCountForEtcdNodeSets(replicaSets []*kubernetesimalv1alpha1.EtcdNodeSet) int32 {
	totalActualReplicas := int32(0)
	for _, rs := range replicaSets {
		if rs != nil {
			totalActualReplicas += rs.Status.Replicas
		}
	}
	return totalActualReplicas
}

// getReadyReplicaCountForEtcdNodeSets returns the number of ready pods corresponding to the given replica sets.
func getReadyReplicaCountForEtcdNodeSets(replicaSets []*kubernetesimalv1alpha1.EtcdNodeSet) int32 {
	totalReadyReplicas := int32(0)
	for _, rs := range replicaSets {
		if rs != nil {
			totalReadyReplicas += rs.Status.ReadyReplicas
		}
	}
	return totalReadyReplicas
}

func scaleEtcdNodeSet(
	ctx context.Context,
	c client.Client,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	set *kubernetesimalv1alpha1.EtcdNodeSet,
	newReplicas int32,
) (bool, *kubernetesimalv1alpha1.EtcdNodeSet, error) {
	if *(set.Spec.Replicas) == newReplicas {
		return false, set, nil
	}
	op, newSet, err := k8s_etcdnodeset.Reconcile(
		ctx,
		c,
		set.Name,
		set.Namespace,
		withDesiredReplicasAnnotation(*spec.Replicas),
		withMaxReplicasAnnotation(*(spec.Replicas)+maxSurge(spec)),
		k8s_etcdnodeset.WithReplicas(newReplicas),
		k8s_etcdnodeset.WithSelector(set.Spec.Selector),
		k8s_etcdnodeset.WithTemplate(set.Spec.Template),
	)
	if err != nil {
		return false, nil, fmt.Errorf("unable to scale EtcdNodeSet: %w", err)
	}
	return op != controllerutil.OperationResultNone, newSet, nil
}

// getEtcdNodeCountForEtcdNodeSets returns the sum of Replicas of the given EtcdNodeSets.
func getEtcdNodeCountForEtcdNodeSets(
	replicaSets []*kubernetesimalv1alpha1.EtcdNodeSet,
) int32 {
	totalReplicas := int32(0)
	for _, set := range replicaSets {
		if set != nil {
			totalReplicas += *(set.Spec.Replicas)
		}
	}
	return totalReplicas
}

// maxUnavailable returns the maximum unavailable pods a rolling deployment can take.
func maxUnavailable(spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec) int32 {
	if *(spec.Replicas) == 0 {
		return int32(0)
	}
	// Error caught by validation
	_, maxUnavailable, _ := resolveFenceposts(
		spec.RollingUpdate.MaxSurge,
		spec.RollingUpdate.MaxUnavailable,
		*(spec.Replicas),
	)
	if maxUnavailable > *spec.Replicas {
		return *spec.Replicas
	}
	return maxUnavailable
}

// cleanupUnhealthyReplicas will scale down old EtcdNodeSets with unhealthy replicas, so that all unhealthy replicas will be deleted.
func cleanupUnhealthyReplicas(
	ctx context.Context,
	c client.Client,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	oldSets []*kubernetesimalv1alpha1.EtcdNodeSet,
	maxCleanupCount int32,
) ([]*kubernetesimalv1alpha1.EtcdNodeSet, int32, error) {
	logger := log.FromContext(ctx)

	sort.Sort(etcdNodeSetsByCreationTimestamp(oldSets))
	// Safely scale down all old EtcdNodeSets with unhealthy replicas. EtcdNodeSet will sort the pods in the order
	// such that not-ready < ready, unscheduled < scheduled, and pending < running. This ensures that unhealthy replicas
	// will been deleted first and won't increase unavailability.
	totalScaledDown := int32(0)
	for i, targetSet := range oldSets {
		if totalScaledDown >= maxCleanupCount {
			break
		}
		if *(targetSet.Spec.Replicas) == 0 {
			// cannot scale down this EtcdNodeSet.
			continue
		}
		logger.V(4).Info(
			"Available EtcdNodes are found.",
			"availableReplicas", targetSet.Status.AvailableReplicas,
			"etcdNodeSet", client.ObjectKeyFromObject(targetSet).String(),
		)
		if *(targetSet.Spec.Replicas) == targetSet.Status.AvailableReplicas {
			// no unhealthy replicas found, no scaling required.
			continue
		}

		scaledDownCount := int32(integer.IntMin(
			int(maxCleanupCount-totalScaledDown),
			int(*(targetSet.Spec.Replicas)-targetSet.Status.AvailableReplicas),
		))
		newReplicasCount := *(targetSet.Spec.Replicas) - scaledDownCount
		if newReplicasCount > *(targetSet.Spec.Replicas) {
			return nil, 0, fmt.Errorf(
				"when cleaning up unhealthy replicas, got invalid request to scale down %s/%s %d -> %d",
				targetSet.Namespace, targetSet.Name, *(targetSet.Spec.Replicas), newReplicasCount,
			)
		}
		_, updatedOldSet, err := scaleEtcdNodeSet(ctx, c, spec, targetSet, newReplicasCount)
		if err != nil {
			return nil, totalScaledDown, err
		}
		totalScaledDown += scaledDownCount
		oldSets[i] = updatedOldSet
	}
	return oldSets, totalScaledDown, nil
}

// filterActiveEtcdNodeSets returns EtcdNodeSets that have (or at least ought to have) EtcdNodes.
func filterActiveEtcdNodeSets(sets []*kubernetesimalv1alpha1.EtcdNodeSet) []*kubernetesimalv1alpha1.EtcdNodeSet {
	activeFilter := func(set *kubernetesimalv1alpha1.EtcdNodeSet) bool {
		return set != nil && *(set.Spec.Replicas) > 0
	}
	var filtered []*kubernetesimalv1alpha1.EtcdNodeSet
	for i := range sets {
		if activeFilter(sets[i]) {
			filtered = append(filtered, sets[i])
		}
	}
	return filtered
}

func filterAliveEtcdNodeSets(sets []*kubernetesimalv1alpha1.EtcdNodeSet) []*kubernetesimalv1alpha1.EtcdNodeSet {
	aliveFilter := func(set *kubernetesimalv1alpha1.EtcdNodeSet) bool {
		return set != nil && set.ObjectMeta.DeletionTimestamp == nil
	}
	var filtered []*kubernetesimalv1alpha1.EtcdNodeSet
	for i := range sets {
		if aliveFilter(sets[i]) {
			filtered = append(filtered, sets[i])
		}
	}
	return filtered
}

// scaleDownOldReplicaSetsForRollingUpdate scales down old EtcdNodeSets when deployment strategy is "RollingUpdate".
// Need check maxUnavailable to ensure availability
func scaleDownOldReplicaSetsForRollingUpdate(
	ctx context.Context,
	c client.Client,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	allSets, oldSets []*kubernetesimalv1alpha1.EtcdNodeSet,
) (int32, error) {
	logger := log.FromContext(ctx)

	maxUnavailable := maxUnavailable(spec)

	// Check if we can scale down.
	minAvailable := *(spec.Replicas) - maxUnavailable
	// Find the number of available pods.
	availableNodesCount := getAvailableReplicaCountForEtcdNodeSets(ctx, allSets)
	if availableNodesCount <= minAvailable {
		// Cannot scale down.
		return 0, nil
	}
	logger.V(4).Info(
		"Available EtcdNodes are found",
		"count", availableNodesCount,
	)

	sort.Sort(etcdNodeSetsByCreationTimestamp(oldSets))

	totalScaledDown := int32(0)
	totalScaleDownCount := availableNodesCount - minAvailable
	for _, targetSet := range oldSets {
		if totalScaledDown >= totalScaleDownCount {
			// No further scaling required.
			break
		}
		if *(targetSet.Spec.Replicas) == 0 {
			// cannot scale down this EtcdNodeSet.
			continue
		}
		// Scale down.
		scaleDownCount := int32(integer.IntMin(
			int(*(targetSet.Spec.Replicas)),
			int(totalScaleDownCount-totalScaledDown),
		))
		newReplicasCount := *(targetSet.Spec.Replicas) - scaleDownCount
		if newReplicasCount > *(targetSet.Spec.Replicas) {
			return 0, fmt.Errorf(
				"when scaling down old EtcdNodeSet, got invalid request to scale down %s/%s %d -> %d",
				targetSet.Namespace, targetSet.Name,
				*(targetSet.Spec.Replicas),
				newReplicasCount,
			)
		}
		_, _, err := scaleEtcdNodeSet(ctx, c, spec, targetSet, newReplicasCount)
		if err != nil {
			return totalScaledDown, err
		}

		totalScaledDown += scaleDownCount
	}

	return totalScaledDown, nil
}

// ReplicaSetsByRevision sorts a list of EtcdNodeSet by revision, using their creation timestamp or name as a tie breaker.
// By using the creation timestamp, this sorts from old to new EtcdNodeSets.
type etcdNodeSetsByRevision []*kubernetesimalv1alpha1.EtcdNodeSet

func (o etcdNodeSetsByRevision) Len() int      { return len(o) }
func (o etcdNodeSetsByRevision) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o etcdNodeSetsByRevision) Less(i, j int) bool {
	revision1, err1 := revision(o[i])
	revision2, err2 := revision(o[j])
	if err1 != nil || err2 != nil || revision1 == revision2 {
		return etcdNodeSetsByCreationTimestamp(o).Less(i, j)
	}
	return revision1 < revision2
}

func finalizeEtcdNodeSets(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	status *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus,
) (*kubernetesimalv1alpha1.EtcdNodeDeploymentStatus, error) {
	return status, nil
}
