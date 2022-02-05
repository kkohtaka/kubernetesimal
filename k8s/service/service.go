package service

import (
	"context"
	"fmt"

	"github.com/kkohtaka/kubernetesimal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
	opRes, err := ctrl.CreateOrUpdate(ctx, c, &service, func() error {
		return ctrl.SetControllerReference(owner, &service, scheme)
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create Service %s: %w", k8s.ObjectName(&service.ObjectMeta), err)
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", service.Namespace,
		"name", service.Name,
	)
	switch opRes {
	case controllerutil.OperationResultCreated:
		logger.Info("Service was created")
	case controllerutil.OperationResultUpdated:
		logger.Info("Service was updated")
	}

	return &service, nil
}

func GetAddressFromServiceRef(
	ctx context.Context,
	c client.Client,
	namespace string,
	portName string,
	ref *corev1.LocalObjectReference,
) (string, error) {
	var service corev1.Service
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      ref.Name,
	}
	if err := c.Get(ctx, key, &service); err != nil {
		return "", fmt.Errorf("unable to get Service %s: %w", key, err)
	}

	var port int
	for i := range service.Spec.Ports {
		if service.Spec.Ports[i].Name == portName {
			port = int(service.Spec.Ports[i].Port)
			break
		}
	}
	if port == 0 {
		return "", fmt.Errorf("unable to find a name %q of a port", portName)
	}
	return fmt.Sprintf("%s:%d", service.Spec.ClusterIP, port), nil
}
