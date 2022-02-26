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

package controllers

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
)

// EtcdNodeReconciler reconciles a EtcdNode object
type EtcdNodeReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Name string
}

//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *EtcdNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcdnode", req.NamespacedName)
	ctx = log.IntoContext(ctx, logger)
	var span trace.Span
	ctx, span = otel.Tracer(r.Name).Start(ctx, "Reconcile")
	defer span.End()

	var en kubernetesimalv1alpha1.EtcdNode
	if err := r.Get(ctx, req.NamespacedName, &en); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	status, err := r.doReconcile(ctx, &en, en.Spec, en.Status)
	if statusUpdateErr := r.updateStatus(ctx, &en, status); statusUpdateErr != nil {
		logger.Error(statusUpdateErr, "unable to update a status of an object")
	}
	if err != nil {
		if ShouldRequeue(err) {
			delay := GetDelay(err)
			logger.Info(
				"Reconciliation will be requeued.",
				"reason", err,
				"delay", delay,
			)
			return ctrl.Result{
				RequeueAfter: delay,
			}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *EtcdNodeReconciler) doReconcile(
	ctx context.Context,
	en *kubernetesimalv1alpha1.EtcdNode,
	spec kubernetesimalv1alpha1.EtcdNodeSpec,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
) (kubernetesimalv1alpha1.EtcdNodeStatus, error) {
	ctx, span := otel.Tracer(r.Name).Start(ctx, "doReconcile")
	defer span.End()

	if en.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(en, finalizerName) {
			controllerutil.AddFinalizer(en, finalizerName)
			if err := r.Update(ctx, en); err != nil {
				if apierrors.IsConflict(err) {
					return status, NewRequeueError("conflict").Wrap(err)
				}
				return status, err
			}
			return status, NewRequeueError("finalizer was set").WithDelay(time.Second)
		}
	} else {
		if controllerutil.ContainsFinalizer(en, finalizerName) {
			if newStatus, err := r.finalizeExternalResources(ctx, en, status); err != nil {
				return newStatus, err
			} else {
				status = newStatus
			}

			controllerutil.RemoveFinalizer(en, finalizerName)
			if err := r.Update(ctx, en); err != nil {
				return status, err
			}
			return status, NewRequeueError("finalizer was unset").WithDelay(time.Second)
		}
		return status, nil
	}

	if newStatus, err := r.reconcileExternalResources(ctx, en, en.Spec, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}
	return status, nil
}

func (r *EtcdNodeReconciler) finalizeExternalResources(
	ctx context.Context,
	en *kubernetesimalv1alpha1.EtcdNode,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
) (kubernetesimalv1alpha1.EtcdNodeStatus, error) {
	var span trace.Span
	ctx, span = otel.Tracer(r.Name).Start(ctx, "finalizeExternalResources")
	defer span.End()

	if newStatus, err := finalizeVirtualMachineInstance(ctx, r.Client, en, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	return status, nil
}

func (r *EtcdNodeReconciler) reconcileExternalResources(
	ctx context.Context,
	en *kubernetesimalv1alpha1.EtcdNode,
	spec kubernetesimalv1alpha1.EtcdNodeSpec,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
) (kubernetesimalv1alpha1.EtcdNodeStatus, error) {
	var span trace.Span
	ctx, span = otel.Tracer(r.Name).Start(ctx, "reconcileExternalResources")
	defer span.End()
	logger := log.FromContext(ctx)

	if serviceRef, err := r.reconcilePeerService(ctx, en, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a peer service: %w", err)
	} else {
		status.PeerServiceRef = serviceRef
	}

	if userDataRef, err := r.reconcileUserData(ctx, en, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a userdata: %w", err)
	} else {
		status.UserDataRef = userDataRef
	}

	if vmiRef, err := r.reconcileVirtualMachineInstance(ctx, en, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a virtual machine instance: %w", err)
	} else {
		status.VirtualMachineRef = vmiRef
	}

	if status.LastProvisionedTime.IsZero() {
		if err := r.provisionEtcdMember(ctx, en, spec, status); err != nil {
			return status, fmt.Errorf("unable to provision an etcd member: %w", err)
		}
		status.LastProvisionedTime = &metav1.Time{Time: time.Now()}
		logger.Info("Provisioning an etcd member was completed.")
	}

	if probed, err := r.probeEtcdMember(ctx, en, spec, status); err != nil {
		return status, fmt.Errorf("unable to probe an etcd member: %w", err)
	} else if !probed {
		status.ProbedSinceTime = nil
		return status, NewRequeueError("waiting for an etcd member ready").WithDelay(probeInterval)
	} else {
		logger.Info("Probing an etcd member was succeeded.")
		if status.ProbedSinceTime.IsZero() {
			status.ProbedSinceTime = &metav1.Time{Time: time.Now()}
		}
	}

	return status, nil
}

func (r *EtcdNodeReconciler) updateStatus(
	ctx context.Context,
	en *kubernetesimalv1alpha1.EtcdNode,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
) error {
	logger := log.FromContext(ctx)

	switch {
	case !en.ObjectMeta.DeletionTimestamp.IsZero():
		status.Phase = kubernetesimalv1alpha1.EtcdNodePhaseDeleting
	case status.LastProvisionedTime.IsZero():
		status.Phase = kubernetesimalv1alpha1.EtcdNodePhaseCreating
	case status.ProbedSinceTime.IsZero():
		status.Phase = kubernetesimalv1alpha1.EtcdNodePhaseProvisioned
	default:
		status.Phase = kubernetesimalv1alpha1.EtcdNodePhaseRunning
	}

	if !apiequality.Semantic.DeepEqual(status, en.Status) {
		patch := client.MergeFrom(en.DeepCopy())
		en.Status = status
		if err := r.Client.Status().Patch(ctx, en, patch); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("status couldn't be applied a patch: %w", err)
		}
		logger.V(4).Info("Status was updated.")
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EtcdNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubernetesimalv1alpha1.EtcdNode{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&kubevirtv1.VirtualMachineInstance{}).
		Complete(r)
}
