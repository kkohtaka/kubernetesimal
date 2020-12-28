/*
Copyright 2020 Kazumasa Kohtaka <kkohtaka@gmail.com>.

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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/client-go/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
)

// EtcdReconciler reconciles a Etcd object
type EtcdReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/status,verbs=get;update;patch

// Reconcile reconciles Etcd resources.
func (r *EtcdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("etcd", req.NamespacedName)

	var e kubernetesimalv1alpha1.Etcd
	if err := r.Get(ctx, req.NamespacedName, &e); err != nil {
		log.Error(err, "unable to fetch Etcd")
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
		log.Info(
			string(opRes),
			"VirtualMachineInstance", types.NamespacedName{Namespace: vmi.Namespace, Name: vmi.Name},
		)
	}

	if e.Status.VirtualMachineRef == "" {
		e.Status.VirtualMachineRef = vmi.Name
		if err := r.Client.Status().Update(ctx, &e); err != nil {
			log.Error(err, "unable to update status")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager setups the Etcd controller with controller-runtime's manager.
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
			Name:      e.Name,
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
