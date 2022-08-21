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
	"fmt"

	"go.opentelemetry.io/otel/trace"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/errors"
	"github.com/kkohtaka/kubernetesimal/controller/expectations"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

// Reconciler reconciles a EtcdNodeSet object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Tracer trace.Tracer

	Expectations *expectations.UIDTrackingControllerExpectations
}

//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes/status,verbs=get

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcdnodeset", req.NamespacedName)
	ctx = log.IntoContext(ctx, logger)

	ctx = tracing.NewContext(ctx, r.Tracer)
	tracer := tracing.FromContext(ctx)

	var span trace.Span
	ctx, span = tracer.Start(ctx, "Reconcile")
	defer span.End()

	var ens kubernetesimalv1alpha1.EtcdNodeSet
	if err := r.Get(ctx, req.NamespacedName, &ens); err != nil {
		if apierrors.IsNotFound(err) {
			r.Expectations.DeleteExpectations(req.NamespacedName.String())
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
	spec *kubernetesimalv1alpha1.EtcdNodeSetSpec,
	status *kubernetesimalv1alpha1.EtcdNodeSetStatus,
) (*kubernetesimalv1alpha1.EtcdNodeSetStatus, error) {
	ctx, span := tracing.FromContext(ctx).Start(ctx, "doReconcile")
	defer span.End()

	if !obj.GetDeletionTimestamp().IsZero() {
		return status, nil
	}

	if newStatus, err := r.reconcileExternalResources(ctx, obj, spec, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}
	return status, nil
}

func (r *Reconciler) reconcileExternalResources(
	ctx context.Context,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeSetSpec,
	status *kubernetesimalv1alpha1.EtcdNodeSetStatus,
) (*kubernetesimalv1alpha1.EtcdNodeSetStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileExternalResources")
	defer span.End()

	if newStatus, err := reconcileEtcdNodes(ctx, r.Client, r.Scheme, obj, spec, status, r.Expectations); err != nil {
		return status, fmt.Errorf("unable to reconcile EtcdNodes: %w", err)
	} else {
		status = newStatus
	}
	return status, nil
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	set *kubernetesimalv1alpha1.EtcdNodeSet,
	status *kubernetesimalv1alpha1.EtcdNodeSetStatus,
) error {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "updateStatus")
	defer span.End()

	logger := log.FromContext(ctx)

	if !apiequality.Semantic.DeepEqual(status, &set.Status) {
		patch := client.MergeFrom(set.DeepCopy())
		status.DeepCopyInto(&set.Status)
		if err := r.Client.Status().Patch(ctx, set, patch); err != nil {
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
		Named("etcdnodeset-reconciler").
		For(
			&kubernetesimalv1alpha1.EtcdNodeSet{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Owns(
			&kubernetesimalv1alpha1.EtcdNode{},
			builder.WithPredicates(
				predicate.ResourceVersionChangedPredicate{},
				predicate.Funcs{
					CreateFunc: func(ce event.CreateEvent) bool {
						if ownerRef := metav1.GetControllerOf(ce.Object); ownerRef != nil {
							r.Expectations.CreationObserved(
								client.ObjectKey{
									Namespace: ce.Object.GetNamespace(),
									Name:      ownerRef.Name,
								}.String(),
							)
						}
						return true
					},
					DeleteFunc: func(de event.DeleteEvent) bool {
						if ownerRef := metav1.GetControllerOf(de.Object); ownerRef != nil {
							r.Expectations.DeletionObserved(
								client.ObjectKey{
									Namespace: de.Object.GetNamespace(),
									Name:      ownerRef.Name,
								}.String(),
								client.ObjectKeyFromObject(de.Object).String(),
							)
						}
						return true
					},
				},
			),
		).
		Complete(r)
}
