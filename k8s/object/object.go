package k8s

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

type ObjectOption func(runtime.Object) error

func WithLabel(key, value string) ObjectOption {
	return func(o runtime.Object) error {
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		labels := meta.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[key] = value
		meta.SetLabels(labels)
		return nil
	}
}

func WithOwner(owner metav1.Object, scheme *runtime.Scheme) ObjectOption {
	return func(o runtime.Object) error {
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		return ctrl.SetControllerReference(owner, meta, scheme)
	}
}

func ObjectName(o *metav1.ObjectMeta) string {
	if o.Namespace == "" {
		return o.Name
	}
	return o.Namespace + "/" + o.Name
}
