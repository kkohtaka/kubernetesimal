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

	"go.opentelemetry.io/otel/trace"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

const (
	probeInterval = 5 * time.Second
)

// EtcdNodeProber reconciles a EtcdNode object
type EtcdNodeProber struct {
	client.Client
	Scheme *runtime.Scheme

	Tracer trace.Tracer
}

//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *EtcdNodeProber) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcdnode", req.NamespacedName)
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

	status, err := r.doReconcile(ctx, &en, en.Spec, en.Status)
	if statusUpdateErr := r.updateStatus(ctx, &en, status); statusUpdateErr != nil {
		logger.Error(statusUpdateErr, "unable to update a status of an object")
		return ctrl.Result{}, statusUpdateErr
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: probeInterval}, nil
}

func (r *EtcdNodeProber) doReconcile(
	ctx context.Context,
	en *kubernetesimalv1alpha1.EtcdNode,
	spec kubernetesimalv1alpha1.EtcdNodeSpec,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
) (kubernetesimalv1alpha1.EtcdNodeStatus, error) {
	ctx, span := tracing.FromContext(ctx).Start(ctx, "doReconcile")
	defer span.End()
	logger := log.FromContext(ctx)

	if !en.ObjectMeta.DeletionTimestamp.IsZero() {
		return status, nil
	}

	if !status.IsProvisioned() {
		return status, nil
	}

	if probed, err := probeEtcdMember(ctx, r.Client, en, spec, status); err != nil {
		status.WithReady(false, err.Error()).DeepCopyInto(&status)
		return status, fmt.Errorf("unable to probe an etcd member: %w", err)
	} else {
		if probed {
			logger.V(4).Info("Probing an etcd member was succeeded.")
		} else {
			logger.Info("Probing an etcd member was failed.")
		}
		status.WithReady(probed, "").DeepCopyInto(&status)
	}
	return status, nil
}

func (r *EtcdNodeProber) updateStatus(
	ctx context.Context,
	en *kubernetesimalv1alpha1.EtcdNode,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
) error {
	logger := log.FromContext(ctx)

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
func (r *EtcdNodeProber) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("etcdnode-prober").
		For(&kubernetesimalv1alpha1.EtcdNode{}).
		Complete(r)
}
