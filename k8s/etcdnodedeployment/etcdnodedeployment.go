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

package etcdnodedeployment

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/trace"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

func WithReplicas(replicas int32) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		deployment, ok := o.(*kubernetesimalv1alpha1.EtcdNodeDeployment)
		if !ok {
			return errors.New("not a instance of EtcdNodeDeployment")
		}
		deployment.Spec.Replicas = &replicas
		return nil
	}
}

func WithSelector(selector *metav1.LabelSelector) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		deployment, ok := o.(*kubernetesimalv1alpha1.EtcdNodeDeployment)
		if !ok {
			return errors.New("not a instance of EtcdNodeDeployment")
		}
		deployment.Spec.Selector = selector
		return nil
	}
}

func WithTemplate(template *kubernetesimalv1alpha1.EtcdNodeTemplateSpec) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		deployment, ok := o.(*kubernetesimalv1alpha1.EtcdNodeDeployment)
		if !ok {
			return errors.New("not a instance of EtcdNodeDeployment")
		}
		template.DeepCopyInto(&deployment.Spec.Template)
		return nil
	}
}

func WithRollingUpdate(rollingUpdate *kubernetesimalv1alpha1.RollingUpdateEtcdNodeDeployment) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		deployment, ok := o.(*kubernetesimalv1alpha1.EtcdNodeDeployment)
		if !ok {
			return errors.New("not a instance of EtcdNodeDeployment")
		}
		rollingUpdate.DeepCopyInto(&deployment.Spec.RollingUpdate)
		return nil
	}
}

func WithRevisionHistoryLimit(limit *int32) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		deployment, ok := o.(*kubernetesimalv1alpha1.EtcdNodeDeployment)
		if !ok {
			return errors.New("not a instance of EtcdNodeDeployment")
		}
		deployment.Spec.RevisionHistoryLimit = limit
		return nil
	}
}

func Create(
	ctx context.Context,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (controllerutil.OperationResult, *kubernetesimalv1alpha1.EtcdNodeDeployment, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "Create")
	defer span.End()

	logger := log.FromContext(ctx).WithValues(
		"namespace", namespace,
		"name", name,
	)

	var deployment kubernetesimalv1alpha1.EtcdNodeDeployment
	deployment.Name = name
	deployment.Namespace = namespace

	for _, fn := range opts {
		if err := fn(&deployment); err != nil {
			return controllerutil.OperationResultNone, nil, err
		}
	}

	if err := c.Create(ctx, &deployment); err != nil {
		return controllerutil.OperationResultNone, nil, fmt.Errorf("unable to create EtcdNodeDeployment: %w", err)
	}

	logger.Info("EtcdNodeDeployment was created.")
	return controllerutil.OperationResultCreated, &deployment, nil
}

func Update(
	ctx context.Context,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (controllerutil.OperationResult, *kubernetesimalv1alpha1.EtcdNodeDeployment, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "Update")
	defer span.End()

	logger := log.FromContext(ctx).WithValues(
		"namespace", namespace,
		"name", name,
	)

	var deployment kubernetesimalv1alpha1.EtcdNodeDeployment
	deployment.Name = name
	deployment.Namespace = namespace
	if err := c.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment); err != nil {
		return controllerutil.OperationResultNone, nil, fmt.Errorf("unable to get EtcdNodeDeployment: %w", err)
	}

	existing := deployment.DeepCopyObject()
	for _, fn := range opts {
		if err := fn(&deployment); err != nil {
			return controllerutil.OperationResultNone, nil, err
		}
	}
	if apiequality.Semantic.DeepEqual(existing, &deployment) {
		return controllerutil.OperationResultNone, &deployment, nil
	}

	if err := c.Update(ctx, &deployment); err != nil {
		return controllerutil.OperationResultNone, nil, fmt.Errorf("unable to create EtcdNodeDeployment: %w", err)
	}

	logger.Info("EtcdNodeDeployment was updated.")
	return controllerutil.OperationResultUpdated, &deployment, nil
}

func Reconcile(
	ctx context.Context,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (controllerutil.OperationResult, *kubernetesimalv1alpha1.EtcdNodeDeployment, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "Reconcile")
	defer span.End()

	logger := log.FromContext(ctx).WithValues(
		"namespace", namespace,
		"name", name,
	)

	var deployment kubernetesimalv1alpha1.EtcdNodeDeployment
	deployment.Name = name
	deployment.Namespace = namespace

	opRes, err := ctrl.CreateOrUpdate(ctx, c, &deployment, func() error {
		for _, fn := range opts {
			if err := fn(&deployment); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return controllerutil.OperationResultNone, nil, fmt.Errorf(
			"unable to create or update EtcdNodeDeployment %s: %w",
			k8s_object.ObjectName(&deployment.ObjectMeta),
			err,
		)
	}

	switch opRes {
	case controllerutil.OperationResultCreated:
		logger.Info("EtcdNodeDeployment was created.")
	case controllerutil.OperationResultUpdated:
		logger.Info("EtcdNodeDeployment was updated.")
	case controllerutil.OperationResultNone:
		logger.V(4).Info("EtcdNodeDeployment was unchanged.")
	}

	return opRes, &deployment, nil
}
