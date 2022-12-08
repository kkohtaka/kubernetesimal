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

package etcdnode

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/errors"
	"github.com/kkohtaka/kubernetesimal/controller/finalizer"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

// Reconciler reconciles a EtcdNode object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Tracer trace.Tracer
}

//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances/status,verbs=get
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcd-node", req.NamespacedName)
	ctx = log.IntoContext(ctx, logger)
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "Reconcile")
	defer span.End()

	var en kubernetesimalv1alpha1.EtcdNode
	if err := r.Get(ctx, req.NamespacedName, &en); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	status, err := r.doReconcile(ctx, &en, en.Spec.DeepCopy(), en.Status.DeepCopy())
	if statusUpdateErr := r.updateStatus(ctx, &en, status); statusUpdateErr != nil {
		logger.Error(statusUpdateErr, "unable to update a status of an object")
	}
	if err != nil {
		if errors.ShouldRequeue(err) {
			delay := errors.GetDelay(err)
			logger.V(2).Info(
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

func (r *Reconciler) doReconcile(
	ctx context.Context,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeSpec,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) (*kubernetesimalv1alpha1.EtcdNodeStatus, error) {
	ctx, span := tracing.FromContext(ctx).Start(ctx, "doReconcile")
	defer span.End()

	if obj.GetDeletionTimestamp().IsZero() {
		if !finalizer.HasFinalizer(obj) {
			if err := finalizer.SetFinalizer(ctx, r.Client, obj); err != nil {
				if apierrors.IsNotFound(err) {
					return status, nil
				}
				return status, fmt.Errorf("unable to set finalizer: %w", err)
			}
			return status, errors.NewRequeueError("finalizer was set").WithDelay(time.Second)
		}
	} else {
		if finalizer.HasFinalizer(obj) {
			if newStatus, err := r.finalizeExternalResources(ctx, obj, spec, status); err != nil {
				return newStatus, err
			} else {
				status = newStatus
			}

			if err := finalizer.UnsetFinalizer(ctx, r.Client, obj); err != nil {
				if apierrors.IsNotFound(err) {
					return status, nil
				}
				return status, fmt.Errorf("unable to unset finalizer: %w", err)
			}
			return status, errors.NewRequeueError("finalizer was unset").WithDelay(time.Second)
		}
		return status, nil
	}

	if newStatus, err := r.reconcileExternalResources(ctx, obj, spec, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}
	return status, nil
}

func (r *Reconciler) finalizeExternalResources(
	ctx context.Context,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeSpec,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) (*kubernetesimalv1alpha1.EtcdNodeStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizeExternalResources")
	defer span.End()

	if newStatus, err := finalizeEtcdMember(ctx, r.Client, obj, spec, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	if newStatus, err := finalizeVirtualMachineInstance(ctx, r.Client, obj, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	return status, nil
}

func (r *Reconciler) reconcileExternalResources(
	ctx context.Context,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeSpec,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) (*kubernetesimalv1alpha1.EtcdNodeStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileExternalResources")
	defer span.End()
	logger := log.FromContext(ctx)

	if serviceRef, err := reconcilePeerService(ctx, r.Client, r.Scheme, obj, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a peer service: %w", err)
	} else {
		status.PeerServiceRef = serviceRef
	}

	if userDataRef, err := reconcileUserData(ctx, r.Client, r.Scheme, obj, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a userdata: %w", err)
	} else {
		status.UserDataRef = userDataRef
	}

	if vmiRef, err := reconcileVirtualMachineInstance(ctx, r.Client, r.Scheme, obj, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a virtual machine instance: %w", err)
	} else {
		status.VirtualMachineInstanceRef = vmiRef
	}

	if !status.IsProvisioned() {
		if err := provisionEtcdMember(ctx, r.Client, obj, spec, status); err != nil {
			status.WithProvisioned(false, err.Error()).DeepCopyInto(status)
			return status, fmt.Errorf("unable to provision an etcd member: %w", err)
		}
		status.WithProvisioned(true, "").DeepCopyInto(status)
		logger.Info("Provisioning an etcd member was completed.")
	}

	return status, nil
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	en *kubernetesimalv1alpha1.EtcdNode,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) error {
	logger := log.FromContext(ctx)

	switch {
	case !en.ObjectMeta.DeletionTimestamp.IsZero():
		status.Phase = kubernetesimalv1alpha1.EtcdNodePhaseDeleting
	case status.IsReady():
		status.Phase = kubernetesimalv1alpha1.EtcdNodePhaseRunning
	case status.IsReadyOnce():
		status.Phase = kubernetesimalv1alpha1.EtcdNodePhaseError
	case status.IsProvisioned():
		status.Phase = kubernetesimalv1alpha1.EtcdNodePhaseProvisioned
	default:
		status.Phase = kubernetesimalv1alpha1.EtcdNodePhaseCreating
	}

	if !apiequality.Semantic.DeepEqual(status, &en.Status) {
		patch := client.MergeFrom(en.DeepCopy())
		status.DeepCopyInto(&en.Status)
		if err := r.Client.Status().Patch(ctx, en, patch); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("status couldn't be applied a patch: %w", err)
		}
		logger.V(2).Info("Status was updated.")
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("etcdnode-reconciler").
		For(
			&kubernetesimalv1alpha1.EtcdNode{},
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Owns(
			&corev1.Secret{},
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Owns(
			&corev1.Service{},
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Owns(
			&kubevirtv1.VirtualMachineInstance{},
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
