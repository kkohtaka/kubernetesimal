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

package etcdnodeset

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
)

func WithReplicas(replicas int32) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNodeSet)
		if !ok {
			return errors.New("not a instance of EtcdNodeSet")
		}
		node.Spec.Replicas = &replicas
		return nil
	}
}

func WithSelector(selector *metav1.LabelSelector) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNodeSet)
		if !ok {
			return errors.New("not a instance of EtcdNodeSet")
		}
		node.Spec.Selector = selector
		return nil
	}
}

func WithTemplate(template kubernetesimalv1alpha1.EtcdNodeTemplateSpec) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNodeSet)
		if !ok {
			return errors.New("not a instance of EtcdNodeSet")
		}
		node.Spec.Template = template
		return nil
	}
}

func CreateOnlyIfNotExist(
	ctx context.Context,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (controllerutil.OperationResult, *kubernetesimalv1alpha1.EtcdNodeSet, error) {
	var set kubernetesimalv1alpha1.EtcdNodeSet
	set.Name = name
	set.Namespace = namespace

	if err := c.Get(ctx, client.ObjectKeyFromObject(&set), &set); err != nil {
		if apierrors.IsNotFound(err) {
			return Reconcile(ctx, c, name, namespace, opts...)
		} else {
			return controllerutil.OperationResultNone, nil, err
		}
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", set.Namespace,
		"name", set.Name,
	)
	logger.V(4).Info("EtcdNodeSet already exists")

	return controllerutil.OperationResultNone, &set, nil
}

func Reconcile(
	ctx context.Context,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (controllerutil.OperationResult, *kubernetesimalv1alpha1.EtcdNodeSet, error) {
	var set kubernetesimalv1alpha1.EtcdNodeSet
	set.Name = name
	set.Namespace = namespace

	opRes, err := ctrl.CreateOrUpdate(ctx, c, &set, func() error {
		for _, fn := range opts {
			if err := fn(&set); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return controllerutil.OperationResultNone, nil, fmt.Errorf(
			"unable to create or update EtcdNodeSet %s: %w",
			k8s_object.ObjectName(&set.ObjectMeta),
			err,
		)
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", set.Namespace,
		"name", set.Name,
	)
	switch opRes {
	case controllerutil.OperationResultCreated:
		logger.Info("EtcdNodeSet was created.")
	case controllerutil.OperationResultUpdated:
		logger.Info("EtcdNodeSet was updated.")
	case controllerutil.OperationResultNone:
		logger.V(4).Info("EtcdNodeSet was unchanged.")
	}

	return opRes, &set, nil
}
