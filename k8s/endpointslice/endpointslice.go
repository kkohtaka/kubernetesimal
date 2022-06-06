package endpointslice

import (
	"context"
	"errors"
	"fmt"

	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	pointerutils "k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
)

func WithAddressType(addressType discoveryv1beta1.AddressType) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		ep, ok := o.(*discoveryv1beta1.EndpointSlice)
		if !ok {
			return errors.New("not a instance of EndpointSlice")
		}
		ep.AddressType = addressType
		return nil
	}
}

func WithPort(name string, port int32) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		ep, ok := o.(*discoveryv1beta1.EndpointSlice)
		if !ok {
			return errors.New("not a instance of EndpointSlice")
		}
		for i := range ep.Ports {
			if ep.Ports[i].Name != nil && *ep.Ports[i].Name == name {
				ep.Ports[i].Port = pointerutils.Int32(port)
				return nil
			}
		}
		ep.Ports = append(ep.Ports, discoveryv1beta1.EndpointPort{
			Name: pointerutils.StringPtr(name),
			Port: pointerutils.Int32(port),
		})
		return nil
	}
}

func WithEndpoints(endpoints []discoveryv1beta1.Endpoint) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		ep, ok := o.(*discoveryv1beta1.EndpointSlice)
		if !ok {
			return errors.New("not a instance of EndpointSlice")
		}
		ep.Endpoints = endpoints
		return nil
	}
}

func Reconcile(
	ctx context.Context,
	owner metav1.Object,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (*discoveryv1beta1.EndpointSlice, error) {
	var endpointSlice discoveryv1beta1.EndpointSlice
	endpointSlice.Name = name
	endpointSlice.Namespace = namespace
	opRes, err := ctrl.CreateOrUpdate(ctx, c, &endpointSlice, func() error {
		for _, fn := range opts {
			if err := fn(&endpointSlice); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create EndpointSlice %s: %w", k8s_object.ObjectName(&endpointSlice.ObjectMeta), err)
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", endpointSlice.Namespace,
		"name", endpointSlice.Name,
	)
	switch opRes {
	case controllerutil.OperationResultCreated:
		logger.Info("EndpointSlice was created")
	case controllerutil.OperationResultUpdated:
		logger.Info("EndpointSlice was updated")
	}

	return &endpointSlice, nil
}
