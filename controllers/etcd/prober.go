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

package etcd

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/errors"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

// Prober reconciles a EtcdNode object
type Prober struct {
	client.Client
	Scheme *runtime.Scheme

	Tracer trace.Tracer
}

//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds,verbs=get;list;watch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Prober) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcd", req.NamespacedName)
	ctx = log.IntoContext(ctx, logger)
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "Reconcile")
	defer span.End()

	var e kubernetesimalv1alpha1.Etcd
	if err := r.Get(ctx, req.NamespacedName, &e); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	status, err := r.doReconcile(ctx, &e, e.Spec.DeepCopy(), e.Status.DeepCopy())
	if statusUpdateErr := r.updateStatus(ctx, &e, status); statusUpdateErr != nil {
		logger.Error(statusUpdateErr, "unable to update a status of an object")
		return ctrl.Result{}, statusUpdateErr
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
		logger.Error(err, "unable to process probing")
	}
	return ctrl.Result{RequeueAfter: getProbeInterval(status)}, nil
}

func (r *Prober) doReconcile(
	ctx context.Context,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdSpec,
	status *kubernetesimalv1alpha1.EtcdStatus,
) (*kubernetesimalv1alpha1.EtcdStatus, error) {
	ctx, span := tracing.FromContext(ctx).Start(ctx, "doReconcile")
	defer span.End()
	logger := log.FromContext(ctx)

	if !obj.GetDeletionTimestamp().IsZero() {
		logger.V(4).Info("Etcd is being deleted")
		return status, nil
	}

	if probeTime := status.LastReadyProbeTime(); probeTime != nil {
		interval := getProbeInterval(status)
		if time.Since(probeTime.Time) < interval {
			return status, errors.NewRequeueError("the object was probed within the last probe interval").
				WithDelay(interval - time.Since(probeTime.Time))
		}
	}

	if probed, message, err := probeEtcd(ctx, r.Client, obj, spec, status); err != nil {
		status.WithReady(false, err.Error()).DeepCopyInto(status)
		return status, fmt.Errorf("unable to probe an etcd: %w", err)
	} else {
		if probed {
			logger.V(4).Info("Probing an etcd was succeeded.")
		} else {
			logger.V(4).Info("Probing an etcd was failed.")
		}
		status.WithReady(probed, message).DeepCopyInto(status)
	}

	if probed, message, err := probeEtcdMembers(ctx, r.Client, obj, spec, status); err != nil {
		status.WithMembersHealthy(false, err.Error()).DeepCopyInto(status)
		return status, fmt.Errorf("unable to probe etcd members: %w", err)
	} else {
		if probed {
			logger.V(4).Info("Probing etcd members was succeeded.")
		} else {
			logger.V(4).Info("Probing etcd members was failed.")
		}
		status.WithMembersHealthy(probed, message).DeepCopyInto(status)
	}

	return status, nil
}

func (r *Prober) updateStatus(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status *kubernetesimalv1alpha1.EtcdStatus,
) error {
	logger := log.FromContext(ctx)

	if !apiequality.Semantic.DeepEqual(status, &e.Status) {
		patch := client.MergeFrom(e.DeepCopy())
		status.DeepCopyInto(&e.Status)
		if err := r.Client.Status().Patch(ctx, e, patch); err != nil {
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
func (r *Prober) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("etcd-prober").
		For(&kubernetesimalv1alpha1.Etcd{}).
		Complete(r)
}

func getProbeInterval(status *kubernetesimalv1alpha1.EtcdStatus) time.Duration {
	const (
		probeIntervalOnNotReady = 5 * time.Second
		probeInterval           = 3 * time.Minute
	)

	if !status.IsReady() {
		return probeIntervalOnNotReady
	}
	if !status.AreMembersHealthy() {
		return probeIntervalOnNotReady
	}
	return probeInterval
}
