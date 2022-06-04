package controllers

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
)

func isEtcdNodeProvisioned(_ context.Context, status kubernetesimalv1alpha1.EtcdNodeStatus) bool {
	for _, cond := range status.Conditions {
		if cond.Type == kubernetesimalv1alpha1.EtcdNodeConditionTypeProvisioned {
			return !cond.LastProbeTime.IsZero()
		}
	}
	return false
}

func isEtcdNodeReady(_ context.Context, status kubernetesimalv1alpha1.EtcdNodeStatus) bool {
	for _, cond := range status.Conditions {
		if cond.Type == kubernetesimalv1alpha1.EtcdNodeConditionTypeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func isEtcdNodeReadyOnce(_ context.Context, status kubernetesimalv1alpha1.EtcdNodeStatus) bool {
	for _, cond := range status.Conditions {
		if cond.Type == kubernetesimalv1alpha1.EtcdNodeConditionTypeReady {
			return !cond.LastProbeTime.IsZero()
		}
	}
	return false
}

func setEtcdNodeReadyWithMessage(
	ctx context.Context,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
	ready bool,
	message string,
) kubernetesimalv1alpha1.EtcdNodeStatus {
	return setEtcdNodeStatusCondition(
		ctx,
		status,
		kubernetesimalv1alpha1.EtcdNodeConditionTypeReady,
		ready,
		message,
	)
}

func setEtcdNodeProvisionedWithMessage(
	ctx context.Context,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
	provisioned bool,
	message string,
) kubernetesimalv1alpha1.EtcdNodeStatus {
	return setEtcdNodeStatusCondition(
		ctx,
		status,
		kubernetesimalv1alpha1.EtcdNodeConditionTypeProvisioned,
		provisioned,
		message,
	)
}

func setEtcdNodeStatusCondition(
	_ context.Context,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
	conditionType kubernetesimalv1alpha1.EtcdNodeConditionType,
	ready bool,
	message string,
) kubernetesimalv1alpha1.EtcdNodeStatus {
	newStatus := status.DeepCopy()
	now := metav1.NewTime(time.Now())
	condStatus := corev1.ConditionFalse
	if ready {
		condStatus = corev1.ConditionTrue
	}
	for i := range newStatus.Conditions {
		if newStatus.Conditions[i].Type == conditionType {
			if newStatus.Conditions[i].Status != condStatus {
				newStatus.Conditions[i].LastTransitionTime = &now
			}
			if ready {
				newStatus.Conditions[i].LastProbeTime = &now
			}
			newStatus.Conditions[i].Status = condStatus
			newStatus.Conditions[i].Message = message
			return *newStatus
		}
	}
	var lastProbeTime *metav1.Time
	if ready {
		lastProbeTime = &now
	}
	newStatus.Conditions = append(
		newStatus.Conditions,
		kubernetesimalv1alpha1.EtcdNodeCondition{
			Type:               conditionType,
			Status:             condStatus,
			LastProbeTime:      lastProbeTime,
			LastTransitionTime: lastProbeTime,
			Message:            message,
		},
	)
	return *newStatus
}
