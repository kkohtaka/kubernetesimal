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

package service

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
)

func WithType(typ corev1.ServiceType) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		s, ok := o.(*corev1.Service)
		if !ok {
			return errors.New("not a instance of Service")
		}
		s.Spec.Type = typ
		return nil
	}
}

func WithPort(name string, port, targetPort int32) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		s, ok := o.(*corev1.Service)
		if !ok {
			return errors.New("not a instance of Service")
		}
		for i := range s.Spec.Ports {
			if s.Spec.Ports[i].Name == name {
				s.Spec.Ports[i].Port = port
				s.Spec.Ports[i].TargetPort = intstr.FromInt(int(targetPort))
				return nil
			}
		}
		s.Spec.Ports = append(s.Spec.Ports, corev1.ServicePort{
			Name:       name,
			Port:       port,
			TargetPort: intstr.FromInt(int(targetPort)),
		})
		return nil
	}
}

func WithSelector(key, value string) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		s, ok := o.(*corev1.Service)
		if !ok {
			return errors.New("not a instance of Service")
		}
		if s.Spec.Selector == nil {
			s.Spec.Selector = make(map[string]string)
		}
		s.Spec.Selector[key] = value
		return nil
	}
}

func Reconcile(
	ctx context.Context,
	owner metav1.Object,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (*corev1.Service, error) {
	var service corev1.Service
	service.Name = name
	service.Namespace = namespace
	opRes, err := ctrl.CreateOrUpdate(ctx, c, &service, func() error {
		for _, fn := range opts {
			if err := fn(&service); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create Service %s: %w", k8s_object.ObjectName(&service.ObjectMeta), err)
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
