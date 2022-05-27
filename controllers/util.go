package controllers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func keyFromObject(o metav1.Object) string {
	return types.NamespacedName{
		Namespace: o.GetNamespace(),
		Name:      o.GetName(),
	}.String()
}
