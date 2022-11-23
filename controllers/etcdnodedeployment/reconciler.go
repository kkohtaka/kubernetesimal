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
	"time"

	"go.opentelemetry.io/otel/trace"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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

// Reconciler reconciles a EtcdNodeDeployment object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Tracer trace.Tracer
}

//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodedeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodedeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodedeployments/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodesets/status,verbs=get

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcdnodedeployment", req.NamespacedName)
	ctx = log.IntoContext(ctx, logger)

	ctx = tracing.NewContext(ctx, r.Tracer)
	tracer := tracing.FromContext(ctx)

	var span trace.Span
	ctx, span = tracer.Start(ctx, "Reconcile")
	defer span.End()

	var ens kubernetesimalv1alpha1.EtcdNodeDeployment
	if err := r.Get(ctx, req.NamespacedName, &ens); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	status, err := r.doReconcile(ctx, &ens, ens.Spec.DeepCopy(), ens.Status.DeepCopy())
	if statusUpdateErr := r.updateStatus(ctx, &ens, status); statusUpdateErr != nil {
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
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	status *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus,
) (*kubernetesimalv1alpha1.EtcdNodeDeploymentStatus, error) {
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
			if newStatus, err := r.finalizeExternalResources(ctx, obj, status); err != nil {
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
	status *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus,
) (*kubernetesimalv1alpha1.EtcdNodeDeploymentStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizeExternalResources")
	defer span.End()

	if newStatus, err := finalizeEtcdNodeSets(ctx, r.Client, obj, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	return status, nil
}

func (r *Reconciler) reconcileExternalResources(
	ctx context.Context,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeDeploymentSpec,
	status *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus,
) (*kubernetesimalv1alpha1.EtcdNodeDeploymentStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileExternalResources")
	defer span.End()

	if newStatus, err := reconcileEtcdNodeSets(ctx, r.Client, r.Scheme, obj, spec, status); err != nil {
		return status, fmt.Errorf("unable to reconcile EtcdNodes: %w", err)
	} else {
		status = newStatus
	}

	return status, nil
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	ens *kubernetesimalv1alpha1.EtcdNodeDeployment,
	status *kubernetesimalv1alpha1.EtcdNodeDeploymentStatus,
) error {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "updateStatus")
	defer span.End()

	logger := log.FromContext(ctx)

	if !apiequality.Semantic.DeepEqual(status, &ens.Status) {
		patch := client.MergeFrom(ens.DeepCopy())
		status.DeepCopyInto(&ens.Status)
		if err := r.Client.Status().Patch(ctx, ens, patch); err != nil {
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
		Named("etcdnodedeployment-reconciler").
		For(
			&kubernetesimalv1alpha1.EtcdNodeDeployment{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Owns(
			&kubernetesimalv1alpha1.EtcdNodeSet{},
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
