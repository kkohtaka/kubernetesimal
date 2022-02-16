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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var etcdlog = logf.Log.WithName("etcd-resource")

func (r *Etcd) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-kubernetesimal-kkohtaka-org-v1alpha1-etcd,mutating=true,failurePolicy=fail,sideEffects=None,groups=kubernetesimal.kkohtaka.org,resources=etcds,verbs=create;update,versions=v1alpha1,name=metcd.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Etcd{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Etcd) Default() {
	etcdlog.Info("default", "name", r.Name)

	if r.Spec.Replicas == nil {
		r.Spec.Replicas = new(int32)
		*r.Spec.Replicas = 1
	}
}

//+kubebuilder:webhook:path=/validate-kubernetesimal-kkohtaka-org-v1alpha1-etcd,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubernetesimal.kkohtaka.org,resources=etcds,verbs=create;update;delete,versions=v1alpha1,name=vetcd.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Etcd{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Etcd) ValidateCreate() error {
	etcdlog.Info("validate create", "name", r.Name)

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Etcd) ValidateUpdate(old runtime.Object) error {
	etcdlog.Info("validate update", "name", r.Name)

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Etcd) ValidateDelete() error {
	etcdlog.Info("validate delete", "name", r.Name)

	return nil
}
