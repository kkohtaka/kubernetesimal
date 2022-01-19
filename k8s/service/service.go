package service

import (
	"context"
	"fmt"

	"github.com/kkohtaka/kubernetesimal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceOption func(*corev1.Service)

func WithType(typ corev1.ServiceType) ServiceOption {
	return func(s *corev1.Service) {
		s.Spec.Type = typ
	}
}

func WithPort(name string, port, targetPort int32) ServiceOption {
	return func(s *corev1.Service) {
		s.Spec.Ports = append(s.Spec.Ports, corev1.ServicePort{
			Name:       name,
			Port:       port,
			TargetPort: intstr.FromInt(int(targetPort)),
		})
	}
}

func WithSelector(key, value string) ServiceOption {
	return func(s *corev1.Service) {
		if s.Spec.Selector == nil {
			s.Spec.Selector = make(map[string]string)
		}
		s.Spec.Selector[key] = value
	}
}

func Reconcile(
	ctx context.Context,
	owner metav1.Object,
	scheme *runtime.Scheme,
	c client.Client,
	meta *metav1.ObjectMeta,
	opts ...func(*corev1.Service),
) (*corev1.Service, error) {
	var service corev1.Service
	meta.DeepCopyInto(&service.ObjectMeta)
	for _, fn := range opts {
		fn(&service)
	}
	_, err := ctrl.CreateOrUpdate(ctx, c, &service, func() error {
		return ctrl.SetControllerReference(owner, &service, scheme)
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create Service %s: %w", k8s.ObjectName(&service.ObjectMeta), err)
	}
	return &service, nil
}
