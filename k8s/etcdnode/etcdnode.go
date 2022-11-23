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

package etcdnode

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
)

func WithVersion(version string) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNode)
		if !ok {
			return errors.New("not a instance of EtcdNode")
		}
		node.Spec.Version = version
		return nil
	}
}

func WithCACertificateRef(caCertificateRef corev1.SecretKeySelector) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNode)
		if !ok {
			return errors.New("not a instance of EtcdNode")
		}
		node.Spec.CACertificateRef = caCertificateRef
		return nil
	}
}

func WithCAPrivateKeyRef(caPrivateKeyRef corev1.SecretKeySelector) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNode)
		if !ok {
			return errors.New("not a instance of EtcdNode")
		}
		node.Spec.CAPrivateKeyRef = caPrivateKeyRef
		return nil
	}
}

func WithClientCertificateRef(ClientCertificateRef corev1.SecretKeySelector) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNode)
		if !ok {
			return errors.New("not a instance of EtcdNode")
		}
		node.Spec.ClientCertificateRef = ClientCertificateRef
		return nil
	}
}

func WithClientPrivateKeyRef(ClientPrivateKeyRef corev1.SecretKeySelector) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNode)
		if !ok {
			return errors.New("not a instance of EtcdNode")
		}
		node.Spec.ClientPrivateKeyRef = ClientPrivateKeyRef
		return nil
	}
}

func WithSSHPrivateKeyRef(sshPrivateKeyRef corev1.SecretKeySelector) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNode)
		if !ok {
			return errors.New("not a instance of EtcdNode")
		}
		node.Spec.SSHPrivateKeyRef = sshPrivateKeyRef
		return nil
	}
}

func WithSSHPublicKeyRef(sshPublicKeyRef corev1.SecretKeySelector) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNode)
		if !ok {
			return errors.New("not a instance of EtcdNode")
		}
		node.Spec.SSHPublicKeyRef = sshPublicKeyRef
		return nil
	}
}

func WithServiceRef(serviceRef corev1.LocalObjectReference) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNode)
		if !ok {
			return errors.New("not a instance of EtcdNode")
		}
		node.Spec.ServiceRef = serviceRef
		return nil
	}
}

func AsFirstNode(asFirstNode bool) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		node, ok := o.(*kubernetesimalv1alpha1.EtcdNode)
		if !ok {
			return errors.New("not a instance of EtcdNode")
		}
		node.Spec.AsFirstNode = asFirstNode
		return nil
	}
}

func Create(
	ctx context.Context,
	c client.Client,
	opts ...k8s_object.ObjectOption,
) (*kubernetesimalv1alpha1.EtcdNode, error) {
	var node kubernetesimalv1alpha1.EtcdNode
	node.Spec.AsFirstNode = false

	for _, fn := range opts {
		if err := fn(&node); err != nil {
			return nil, err
		}
	}
	if err := c.Create(ctx, &node); err != nil {
		return nil, fmt.Errorf("unable to create EtcdNode %s: %w", k8s_object.ObjectName(&node.ObjectMeta), err)
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", node.Namespace,
		"name", node.Name,
	)
	logger.Info("EtcdNode was created")

	return &node, nil
}

func Update(
	ctx context.Context,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (*kubernetesimalv1alpha1.EtcdNode, error) {
	var node kubernetesimalv1alpha1.EtcdNode
	node.Name = name
	node.Namespace = namespace
	node.Spec.AsFirstNode = false

	for _, fn := range opts {
		if err := fn(&node); err != nil {
			return nil, err
		}
	}
	if err := c.Update(ctx, &node); err != nil {
		return nil, fmt.Errorf("unable to update EtcdNode %s: %w", k8s_object.ObjectName(&node.ObjectMeta), err)
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", node.Namespace,
		"name", node.Name,
	)
	logger.Info("EtcdNode was created")

	return &node, nil
}
