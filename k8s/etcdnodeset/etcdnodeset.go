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
