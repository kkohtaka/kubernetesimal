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

package etcd

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
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

// Reconciler reconciles a Etcd object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Tracer trace.Tracer
}

//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodedeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodedeployments/status,verbs=get
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcd", req.NamespacedName)
	ctx = log.IntoContext(ctx, logger)

	ctx = tracing.NewContext(ctx, r.Tracer)
	tracer := tracing.FromContext(ctx)

	var span trace.Span
	ctx, span = tracer.Start(ctx, "Reconcile")
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
	spec *kubernetesimalv1alpha1.EtcdSpec,
	status *kubernetesimalv1alpha1.EtcdStatus,
) (*kubernetesimalv1alpha1.EtcdStatus, error) {
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
	status *kubernetesimalv1alpha1.EtcdStatus,
) (*kubernetesimalv1alpha1.EtcdStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizeExternalResources")
	defer span.End()

	if err := finalizeEtcdNodeDeployments(ctx, r.Client, obj); err != nil {
		return status, err
	}

	if newStatus, err := finalizeCACertificateSecret(ctx, r.Client, obj, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	if newStatus, err := finalizeClientCertificateSecret(ctx, r.Client, obj, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	if newStatus, err := finalizePeerCertificateSecret(ctx, r.Client, obj, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	if newStatus, err := finalizeSSHKeyPairSecret(ctx, r.Client, obj, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	return status, nil
}

func (r *Reconciler) reconcileExternalResources(
	ctx context.Context,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdSpec,
	status *kubernetesimalv1alpha1.EtcdStatus,
) (*kubernetesimalv1alpha1.EtcdStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileExternalResources")
	defer span.End()

	if certificateRef, privateKeyRef, err := reconcileCACertificate(
		ctx,
		r.Client,
		r.Scheme,
		obj,
		spec,
		status,
	); err != nil {
		return status, fmt.Errorf("unable to prepare a CA certificate: %w", err)
	} else {
		status.CAPrivateKeyRef = privateKeyRef
		status.CACertificateRef = certificateRef
	}

	if certificateRef, privateKeyRef, err := reconcileClientCertificate(
		ctx,
		r.Client,
		r.Scheme,
		obj,
		spec,
		status,
	); err != nil {
		return status, fmt.Errorf("unable to prepare a client certificate: %w", err)
	} else {
		status.ClientPrivateKeyRef = privateKeyRef
		status.ClientCertificateRef = certificateRef
	}

	if certificateRef, privateKeyRef, err := reconcilePeerCertificate(
		ctx,
		r.Client,
		r.Scheme,
		obj,
		spec,
		status,
	); err != nil {
		return status, fmt.Errorf("unable to prepare a certificate for peer communication: %w", err)
	} else {
		status.PeerPrivateKeyRef = privateKeyRef
		status.PeerCertificateRef = certificateRef
	}

	if sshPrivateKeyRef, sshPublicKeyRef, err := reconcileSSHKeyPair(
		ctx,
		r.Client,
		r.Scheme,
		obj,
		spec,
		status,
	); err != nil {
		return status, fmt.Errorf("unable to prepare an SSH key-pair: %w", err)
	} else {
		status.SSHPrivateKeyRef = sshPrivateKeyRef
		status.SSHPublicKeyRef = sshPublicKeyRef
	}

	if serviceRef, err := reconcileService(ctx, r.Client, r.Scheme, obj, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a service: %w", err)
	} else {
		status.ServiceRef = serviceRef
	}

	if endpointSliceRef, err := reconcileEndpointSlice(ctx, r.Client, r.Scheme, obj, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare an endpoint slice: %w", err)
	} else {
		status.EndpointSliceRef = endpointSliceRef
	}

	if deployment, err := reconcileEtcdNodeDeployment(ctx, r.Client, r.Scheme, obj, spec, status); err != nil {
		return nil, fmt.Errorf("unable to prepare EtcdNodeDeployment: %w", err)
	} else {
		status.ReadyReplicas = deployment.Status.ReadyReplicas
	}
	return status, nil
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status *kubernetesimalv1alpha1.EtcdStatus,
) error {
	logger := log.FromContext(ctx)

	if e.Spec.Replicas != nil {
		status.Replicas = *e.Spec.Replicas
	}

	if !e.GetDeletionTimestamp().IsZero() {
		status.Phase = kubernetesimalv1alpha1.EtcdPhaseDeleting
	} else if status.ReadyReplicas != *e.Spec.Replicas {
		if status.IsReadyOnce() && !status.IsReady() {
			status.Phase = kubernetesimalv1alpha1.EtcdPhaseError
		} else {
			status.Phase = kubernetesimalv1alpha1.EtcdPhaseCreating
		}
	} else {
		if status.IsReady() {
			status.Phase = kubernetesimalv1alpha1.EtcdPhaseRunning
		} else {
			status.Phase = kubernetesimalv1alpha1.EtcdPhaseError
		}
	}

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
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("etcd-reconciler").
		For(
			&kubernetesimalv1alpha1.Etcd{},
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
			&kubernetesimalv1alpha1.EtcdNodeDeployment{},
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Owns(
			&kubernetesimalv1alpha1.EtcdNode{},
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
