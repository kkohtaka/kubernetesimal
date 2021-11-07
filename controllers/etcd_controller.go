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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/client-go/api/v1"
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

const (
	finalizerName = "kubernetesimal.kkohtaka.org/finalizer"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *EtcdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcd", req.NamespacedName)
	log.IntoContext(ctx, logger)

	var e kubernetesimalv1alpha1.Etcd
	if err := r.Get(ctx, req.NamespacedName, &e); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch Etcd")
		return ctrl.Result{}, err
	}

	if e.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&e, finalizerName) {
			controllerutil.AddFinalizer(&e, finalizerName)
			if err := r.Update(ctx, &e); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	} else {
		if controllerutil.ContainsFinalizer(&e, finalizerName) {
			if deleted, err := r.deleteExternalResources(ctx, &e); err != nil {
				return ctrl.Result{}, err
			} else if !deleted {
				return ctrl.Result{Requeue: true}, err
			}

			controllerutil.RemoveFinalizer(&e, finalizerName)
			if err := r.Update(ctx, &e); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	status, err := r.reconcileExternalResources(ctx, &e, e.Spec, e.Status)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.updateStatus(ctx, &e, status)
}

func (r *EtcdReconciler) deleteExternalResources(ctx context.Context, e *kubernetesimalv1alpha1.Etcd) (bool, error) {
	key := types.NamespacedName{Name: newVirtualMachineInstanceName(e)}
	var vmi kubevirtv1.VirtualMachineInstance
	if err := r.Client.Get(ctx, key, &vmi); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	if vmi.DeletionTimestamp.IsZero() {
		if err := r.Client.Delete(ctx, &vmi, &client.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
	}
	return false, nil
}

func (r *EtcdReconciler) reconcileExternalResources(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	spec kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	vmi := newVirtualMachineInstance(e)
	_, err := ctrl.CreateOrUpdate(ctx, r.Client, vmi, func() error {
		return ctrl.SetControllerReference(e, &vmi.ObjectMeta, r.Scheme)
	})
	if err != nil {
		return status, fmt.Errorf("unable to create VirtualMachineInstance: %w", err)
	}

	status.VirtualMachineRef = &corev1.LocalObjectReference{
		Name: vmi.Name,
	}

	switch vmi.Status.Phase {
	case kubevirtv1.Running:
		status.Phase = kubernetesimalv1alpha1.EtcdPhaseRunning
	default:
		status.Phase = kubernetesimalv1alpha1.EtcdPhasePending
	}
	return status, nil
}

func (r *EtcdReconciler) updateStatus(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status kubernetesimalv1alpha1.EtcdStatus,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if !reflect.DeepEqual(status, e.Status) {
		patch := client.MergeFrom(e.DeepCopy())
		e.Status = status
		if err := r.Client.Status().Patch(ctx, e, patch); err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			logger.Error(err, "unable to update status")
			return ctrl.Result{}, err
		}
		logger.Info("status is updated")
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EtcdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubernetesimalv1alpha1.Etcd{}).
		Complete(r)
}

var (
	defaultResourceMemoryForEtcd = resource.MustParse("1024M")
)

func newVirtualMachineInstance(e *kubernetesimalv1alpha1.Etcd) *kubevirtv1.VirtualMachineInstance {
	return &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: e.Namespace,
			Name:      newVirtualMachineInstanceName(e),
		},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Domain: kubevirtv1.DomainSpec{
				Resources: kubevirtv1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: defaultResourceMemoryForEtcd,
					},
				},
			},
		},
	}
}

func newVirtualMachineInstanceName(e *kubernetesimalv1alpha1.Etcd) string {
	return e.Name
}
