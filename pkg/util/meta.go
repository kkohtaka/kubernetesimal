package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func NamespacedName(m metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Name:      m.GetName(),
		Namespace: m.GetNamespace(),
	}
}
