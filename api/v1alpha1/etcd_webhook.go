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

package v1alpha1

import (
	"github.com/blang/semver/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
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

var (
	defaultEtcdVersion semver.Version = semver.MustParse("3.5.1")
)

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Etcd) Default() {
	etcdlog.Info("default", "name", r.Name)

	if r.Spec.Version == nil {
		r.Spec.Version = new(string)
		*r.Spec.Version = defaultEtcdVersion.String()
	}

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

	var errs field.ErrorList
	errs = append(errs, r.validateSpecVersion()...)
	if len(errs) > 0 {
		err := apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Etcd"}, r.Name, errs)
		etcdlog.Error(err, "validation error", "name", r.Name)
		return err
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Etcd) ValidateUpdate(old runtime.Object) error {
	etcdlog.Info("validate update", "name", r.Name)

	var errs field.ErrorList
	errs = append(errs, r.validateSpecVersion()...)
	errs = append(errs, r.validateSpecImagePersistentVolumeClaimRef()...)
	if len(errs) > 0 {
		err := apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Etcd"}, r.Name, errs)
		etcdlog.Error(err, "validation error", "name", r.Name)
		return err
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Etcd) ValidateDelete() error {
	etcdlog.Info("validate delete", "name", r.Name)

	return nil
}

func (r *Etcd) validateSpecVersion() field.ErrorList {
	var errs field.ErrorList
	if ver := r.Spec.Version; ver == nil {
		errs = append(errs,
			field.Required(
				field.NewPath("spec", "version"),
				"spec must have a version",
			),
		)
	} else if _, err := semver.Parse(*ver); err != nil {
		errs = append(errs,
			field.Invalid(
				field.NewPath("spec", "version"),
				r.Spec.Version,
				"the version must be a semantic version",
			),
		)
	}
	return errs
}

func (r *Etcd) validateSpecImagePersistentVolumeClaimRef() field.ErrorList {
	var errs field.ErrorList
	if ref := r.Spec.ImagePersistentVolumeClaimRef; ref.Name == "" {
		errs = append(errs,
			field.Required(
				field.NewPath("spec", "imagePersistentVolumeClaimRef"),
				"spec must have an imagePersistentVolumeClaimRef",
			),
		)
	}
	return errs
}
