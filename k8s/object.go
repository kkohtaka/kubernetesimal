package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ObjectMetaOption func(*metav1.ObjectMeta)

func NewObjectMeta(opts ...ObjectMetaOption) *metav1.ObjectMeta {
	var o metav1.ObjectMeta
	for _, fn := range opts {
		fn(&o)
	}
	return &o
}

func WithName(name string) ObjectMetaOption {
	return func(o *metav1.ObjectMeta) {
		o.Name = name
	}
}

func WithNamespace(namespace string) ObjectMetaOption {
	return func(o *metav1.ObjectMeta) {
		o.Namespace = namespace
	}
}

func WithLabel(key, value string) ObjectMetaOption {
	return func(o *metav1.ObjectMeta) {
		if o.Labels == nil {
			o.Labels = make(map[string]string)
		}
		o.Labels[key] = value
	}
}

func ObjectName(o *metav1.ObjectMeta) string {
	if o.Namespace == "" {
		return o.Name
	}
	return o.Namespace + "/" + o.Name
}
