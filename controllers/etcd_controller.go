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

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
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
}

//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

const (
	finalizerName = "kubernetesimal.kkohtaka.org/finalizer"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *EtcdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcd", req.NamespacedName)
	ctx = log.IntoContext(ctx, logger)

	if err := r.doReconcile(ctx, req); err != nil {
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

func (r *EtcdReconciler) doReconcile(ctx context.Context, req ctrl.Request) error {
	logger := log.FromContext(ctx)

	var e kubernetesimalv1alpha1.Etcd
	if err := r.Get(ctx, req.NamespacedName, &e); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if e.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&e, finalizerName) {
			controllerutil.AddFinalizer(&e, finalizerName)
			if err := r.Update(ctx, &e); err != nil {
				if apierrors.IsConflict(err) {
					return NewRequeueError("conflict").Wrap(err)
				}
				return err
			}
			logger.Info("A finalizer was set.")
			return nil
		}
	} else {
		if controllerutil.ContainsFinalizer(&e, finalizerName) {
			status, deleted, err := r.finalizeExternalResources(ctx, &e, e.Status)
			if err != nil {
				return err
			} else if !deleted {
				if err := r.updateStatus(ctx, &e, status); err != nil {
					return err
				}
				return NewRequeueError("waiting for external resources deleted")
			}

			controllerutil.RemoveFinalizer(&e, finalizerName)
			if err := r.Update(ctx, &e); err != nil {
				return err
			}
			logger.Info("The finalizer was unset.")

			if err := r.updateStatus(ctx, &e, status); err != nil {
				return err
			}
		}
		return nil
	}

	status, err := r.reconcileExternalResources(ctx, &e, e.Spec, e.Status)
	if err != nil {
		return err
	}
	if err := r.updateStatus(ctx, &e, status); err != nil {
		return err
	}
	return nil
}

func (r *EtcdReconciler) finalizeExternalResources(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, bool, error) {
	if newStatus, deleted, err := r.finalizeCACertificateSecret(ctx, e, status); err != nil {
		return status, false, err
	} else if !deleted {
		return newStatus, false, nil
	} else {
		status = newStatus
	}

	if newStatus, deleted, err := r.finalizeClientCertificateSecret(ctx, e, status); err != nil {
		return status, false, err
	} else if !deleted {
		return newStatus, false, nil
	} else {
		status = newStatus
	}

	if newStatus, deleted, err := r.finalizeSSHKeyPairSecret(ctx, e, status); err != nil {
		return status, false, err
	} else if !deleted {
		return newStatus, false, nil
	} else {
		status = newStatus
	}

	if newStatus, deleted, err := r.finalizeVirtualMachineInstance(ctx, e, status); err != nil {
		return status, false, err
	} else if !deleted {
		return newStatus, false, nil
	} else {
		status = newStatus
	}

	return status, true, nil
}

func (r *EtcdReconciler) finalizeSecret(
	ctx context.Context,
	namespace, name string,
) (bool, error) {
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues(
		"object", name,
		"resource", "corev1.Secret",
	))
	return r.finalizeObject(ctx, namespace, name, &corev1.Secret{})
}

func (r *EtcdReconciler) finalizeObject(
	ctx context.Context,
	namespace, name string,
	obj client.Object,
) (bool, error) {
	logger := log.FromContext(ctx)

	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if err := r.Client.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("The object has already been deleted.")
			return true, nil
		}
		return false, err
	}
	if obj.GetDeletionTimestamp().IsZero() {
		if err := r.Client.Delete(ctx, obj, &client.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("The object has already been deleted.")
				return true, nil
			}
			return false, err
		}
		logger.Info("The object has started to be deleted.")
	} else {
		logger.Info("The object is beeing deleted.")
	}
	return false, nil
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

	if userDataRef, err := r.reconcileUserData(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a userdata: %w", err)
	} else {
		status.UserDataRef = userDataRef
	}

	if vmiRef, err := r.reconcileVirtualMachineInstance(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a virtual machine instance: %w", err)
	} else {
		status.VirtualMachineRef = vmiRef
	}

	if status.LastProvisionedTime.IsZero() {
		if err := r.provisionEtcdMember(ctx, e, spec, status); err != nil {
			return status, fmt.Errorf("unable to provision an etcd member: %w", err)
		}
		status.LastProvisionedTime = &metav1.Time{Time: time.Now()}
		logger.Info("Provisioning an etcd member was completed.")
	}

	if probed, err := r.probeEtcdMember(ctx, e, spec, status); err != nil {
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

func (r *EtcdReconciler) updateStatus(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status kubernetesimalv1alpha1.EtcdStatus,
) error {
	logger := log.FromContext(ctx)

	switch {
	case !e.ObjectMeta.DeletionTimestamp.IsZero():
		status.Phase = kubernetesimalv1alpha1.EtcdPhaseDeleting
	case e.Status.ProbedSinceTime.IsZero():
		status.Phase = kubernetesimalv1alpha1.EtcdPhasePending
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
			logger.Error(err, "Status couldn't be updated.")
			return err
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
		Owns(&kubevirtv1.VirtualMachineInstance{}).
		Complete(r)
}
