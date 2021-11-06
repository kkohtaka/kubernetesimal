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

	corev1 "k8s.io/api/core/v1"
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
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *EtcdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("parent", req.NamespacedName)

	var e kubernetesimalv1alpha1.Etcd
	if err := r.Get(ctx, req.NamespacedName, &e); err != nil {
		logger.Error(err, "unable to fetch Etcd")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmi := getVirtualMachineInstance(&e)
	opRes, err := ctrl.CreateOrUpdate(ctx, r.Client, vmi, func() error {
		return ctrl.SetControllerReference(&e, &vmi.ObjectMeta, r.Scheme)
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to create VirtualMachineInstance: %w", err)
	}
	if opRes != controllerutil.OperationResultNone {
		logger.Info(
			string(opRes),
			"VirtualMachineInstance", types.NamespacedName{Namespace: vmi.Namespace, Name: vmi.Name},
		)
	}

	e.Status.VirtualMachineRef = vmi.Name

	switch vmi.Status.Phase {
	case kubevirtv1.Running:
		e.Status.Phase = kubernetesimalv1alpha1.EtcdPhaseRunning
	default:
		e.Status.Phase = kubernetesimalv1alpha1.EtcdPhasePending
	}

	if err := r.Client.Status().Update(ctx, &e); err != nil {
		logger.Error(err, "unable to update status")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EtcdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubernetesimalv1alpha1.Etcd{}).
		Owns(&kubevirtv1.VirtualMachineInstance{}).
		Complete(r)
}

var (
	defaultResourceMemoryForEtcd = resource.MustParse("64M")
)

func getVirtualMachineInstance(e *kubernetesimalv1alpha1.Etcd) *kubevirtv1.VirtualMachineInstance {
	return &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: e.Namespace,
			Name:      getVirtualMachineInstanceName(e),
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

func getVirtualMachineInstanceName(e *kubernetesimalv1alpha1.Etcd) string {
	return e.Name
}
