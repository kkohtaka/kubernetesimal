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
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
)

// EtcdReconciler reconciles a Etcd object
type EtcdReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Tracer trace.Tracer
}

//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcdnodes,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *EtcdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcd", req.NamespacedName)
	ctx = log.IntoContext(ctx, logger)
	var span trace.Span
	ctx, span = r.Tracer.Start(ctx, "Reconcile")
	defer span.End()

	var e kubernetesimalv1alpha1.Etcd
	if err := r.Get(ctx, req.NamespacedName, &e); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	status, err := r.doReconcile(ctx, &e, e.Spec, e.Status)
	if statusUpdateErr := r.updateStatus(ctx, &e, status); statusUpdateErr != nil {
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

func (r *EtcdReconciler) doReconcile(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	spec kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	ctx, span := r.Tracer.Start(ctx, "doReconcile")
	defer span.End()

	if e.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(e, finalizerName) {
			controllerutil.AddFinalizer(e, finalizerName)
			if err := r.Update(ctx, e); err != nil {
				if apierrors.IsConflict(err) {
					return status, NewRequeueError("conflict").Wrap(err)
				}
				return status, err
			}
			return status, NewRequeueError("finalizer was set").WithDelay(time.Second)
		}
	} else {
		if controllerutil.ContainsFinalizer(e, finalizerName) {
			if newStatus, err := r.finalizeExternalResources(ctx, e, status); err != nil {
				return newStatus, err
			} else {
				status = newStatus
			}

			controllerutil.RemoveFinalizer(e, finalizerName)
			if err := r.Update(ctx, e); err != nil {
				return status, err
			}
			return status, NewRequeueError("finalizer was unset").WithDelay(time.Second)
		}
		return status, nil
	}

	if newStatus, err := r.reconcileExternalResources(ctx, e, e.Spec, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}
	return status, nil
}

func (r *EtcdReconciler) finalizeExternalResources(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	var span trace.Span
	ctx, span = r.Tracer.Start(ctx, "finalizeExternalResources")
	defer span.End()

	if newStatus, err := r.finalizeCACertificateSecret(ctx, e, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	if newStatus, err := r.finalizeClientCertificateSecret(ctx, e, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	if newStatus, err := r.finalizePeerCertificateSecret(ctx, e, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	if newStatus, err := r.finalizeSSHKeyPairSecret(ctx, e, status); err != nil {
		return newStatus, err
	} else {
		status = newStatus
	}

	return status, nil
}

func finalizeSecret(
	ctx context.Context,
	client client.Client,
	namespace, name string,
) error {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues(
		"object", name,
		"resource", "corev1.Secret",
	))
	return finalizeObject(ctx, client, namespace, name, &corev1.Secret{})
}

func finalizeObject(
	ctx context.Context,
	c client.Client,
	namespace, name string,
	obj client.Object,
) error {
	logger := log.FromContext(ctx)

	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if obj.GetDeletionTimestamp().IsZero() {
		if err := c.Delete(ctx, obj, &client.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		logger.Info("The object has started to be deleted.")
	}
	return NewRequeueError("waiting for an object deleted").WithDelay(5 * time.Second)
}

const (
	probeInterval = 5 * time.Second
)

func (r *EtcdReconciler) reconcileExternalResources(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	spec kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	var span trace.Span
	ctx, span = r.Tracer.Start(ctx, "reconcileExternalResources")
	defer span.End()
	logger := log.FromContext(ctx)

	if certificateRef, privateKeyRef, err := r.reconcileCACertificate(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a CA certificate: %w", err)
	} else {
		status.CAPrivateKeyRef = privateKeyRef
		status.CACertificateRef = certificateRef
	}

	if certificateRef, privateKeyRef, err := r.reconcileClientCertificate(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a client certificate: %w", err)
	} else {
		status.ClientPrivateKeyRef = privateKeyRef
		status.ClientCertificateRef = certificateRef
	}

	if certificateRef, privateKeyRef, err := r.reconcilePeerCertificate(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a certificate for peer communication: %w", err)
	} else {
		status.PeerPrivateKeyRef = privateKeyRef
		status.PeerCertificateRef = certificateRef
	}

	if sshPrivateKeyRef, sshPublicKeyRef, err := r.reconcileSSHKeyPair(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare an SSH key-pair: %w", err)
	} else {
		status.SSHPrivateKeyRef = sshPrivateKeyRef
		status.SSHPublicKeyRef = sshPublicKeyRef
	}

	if serviceRef, err := r.reconcileService(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a service: %w", err)
	} else {
		status.ServiceRef = serviceRef
	}

	var (
		nRunning, nPending int
	)
	for _, nodeRef := range status.NodeRefs {
		var node kubernetesimalv1alpha1.EtcdNode
		if err := r.Get(ctx, types.NamespacedName{Namespace: e.Namespace, Name: nodeRef.Name}, &node); err != nil {
			return status, fmt.Errorf("unable to get an etcd node from reference: %w", err)
		}
		switch node.Status.Phase {
		case kubernetesimalv1alpha1.EtcdNodePhaseRunning:
			nRunning++
		default:
			nPending++
		}
	}
	status.Replicas = int32(nRunning)
	if nPending > 0 {
		logger.V(4).Info("Skip reconciliation since not all nodes are running.")
		return status, nil
	}

	if len(status.NodeRefs) > int(*spec.Replicas) {
		// TODO(kkohtaka): Decrease etcd nodes
		return status, nil
	}

	if len(status.NodeRefs) < int(*spec.Replicas) {
		// TODO(kkohtaka): Fill the proper specification
		node := &kubernetesimalv1alpha1.EtcdNode{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: e.Name + "-",
				Namespace:    e.Namespace,
			},
			Spec: kubernetesimalv1alpha1.EtcdNodeSpec{
				Version: *e.Spec.Version,

				CACertificateRef:     *status.CACertificateRef,
				CAPrivateKeyRef:      *status.CAPrivateKeyRef,
				ClientCertificateRef: *status.ClientCertificateRef,
				ClientPrivateKeyRef:  *status.ClientPrivateKeyRef,
				SSHPrivateKeyRef:     *status.SSHPrivateKeyRef,
				SSHPublicKeyRef:      *status.SSHPublicKeyRef,

				ServiceRef: *status.ServiceRef,
			},
		}
		_, err := ctrl.CreateOrUpdate(ctx, r.Client, node, func() error {
			return ctrl.SetControllerReference(e, node, r.Scheme)
		})
		if err != nil {
			return status, fmt.Errorf("unable to create or update an etcd node: %w", err)
		}
		status.NodeRefs = append(status.NodeRefs, &corev1.LocalObjectReference{Name: node.Name})
		return status, nil
	}

	return status, nil
}

func (r *EtcdReconciler) updateStatus(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status kubernetesimalv1alpha1.EtcdStatus,
) error {
	logger := log.FromContext(ctx)

	switch {
	case !e.ObjectMeta.DeletionTimestamp.IsZero():
		status.Phase = kubernetesimalv1alpha1.EtcdPhaseDeleting
	case status.Replicas != *e.Spec.Replicas:
		status.Phase = kubernetesimalv1alpha1.EtcdPhaseCreating
	default:
		status.Phase = kubernetesimalv1alpha1.EtcdPhaseRunning
	}

	if !apiequality.Semantic.DeepEqual(status, e.Status) {
		patch := client.MergeFrom(e.DeepCopy())
		e.Status = status
		if err := r.Client.Status().Patch(ctx, e, patch); err != nil {
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
func (r *EtcdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubernetesimalv1alpha1.Etcd{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&kubernetesimalv1alpha1.EtcdNode{}).
		Complete(r)
}
