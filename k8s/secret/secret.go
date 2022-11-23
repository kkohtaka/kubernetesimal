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

package k8s

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
)

func WithType(typ corev1.SecretType) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		secret, ok := o.(*corev1.Secret)
		if !ok {
			return errors.New("not a instance of Secret")
		}
		secret.Type = typ
		return nil
	}
}

func WithDataWithKey(key string, value []byte) k8s_object.ObjectOption {
	return func(o runtime.Object) error {
		secret, ok := o.(*corev1.Secret)
		if !ok {
			return errors.New("not a instance of Secret")
		}
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		secret.Data[key] = value
		return nil
	}
}

func CreateOnlyIfNotExist(
	ctx context.Context,
	owner metav1.Object,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (*corev1.Secret, error) {
	var secret corev1.Secret
	secret.Name = name
	secret.Namespace = namespace
	for _, fn := range opts {
		if err := fn(&secret); err != nil {
			return nil, err
		}
	}
	if err := c.Create(ctx, &secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := c.Get(ctx, client.ObjectKeyFromObject(&secret), &secret); err != nil {
				return nil, err
			}
			return &secret, nil
		}
		return nil, fmt.Errorf("unable to create Secret %s: %w", k8s_object.ObjectName(&secret.ObjectMeta), err)
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", secret.Namespace,
		"name", secret.Name,
	)
	logger.Info("Secret was created")

	return &secret, nil
}

func Reconcile(
	ctx context.Context,
	owner metav1.Object,
	c client.Client,
	name, namespace string,
	opts ...k8s_object.ObjectOption,
) (*corev1.Secret, error) {
	var secret corev1.Secret
	secret.Name = name
	secret.Namespace = namespace
	opRes, err := ctrl.CreateOrUpdate(ctx, c, &secret, func() error {
		for _, fn := range opts {
			if err := fn(&secret); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create Secret %s: %w", k8s_object.ObjectName(&secret.ObjectMeta), err)
	}

	logger := log.FromContext(ctx).WithValues(
		"namespace", secret.Namespace,
		"name", secret.Name,
	)
	switch opRes {
	case controllerutil.OperationResultCreated:
		logger.Info("Secret was created")
	case controllerutil.OperationResultUpdated:
		logger.Info("Secret was updated")
	case controllerutil.OperationResultNone:
		logger.V(4).Info("Secret was unchanged")
	}

	return &secret, nil
}

func GetValueFromSecretKeySelector(
	ctx context.Context,
	c client.Client,
	namespace string,
	selector corev1.SecretKeySelector,
) ([]byte, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      selector.LocalObjectReference.Name,
	}
	if err := c.Get(ctx, key, &secret); err != nil {
		return nil, fmt.Errorf("unable to get Secret %s: %w", key, err)
	}
	return secret.Data[selector.Key], nil
}

func GetCertificateFromSecretKeySelector(
	ctx context.Context,
	c client.Client,
	namespace string,
	selector *corev1.SecretKeySelector,
) (*x509.Certificate, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      selector.LocalObjectReference.Name,
	}
	if err := c.Get(ctx, key, &secret); err != nil {
		return nil, fmt.Errorf("unable to get Secret for a certificate: %w", err)
	}

	p, _ := pem.Decode(secret.Data[corev1.TLSCertKey])
	if p == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	cert, err := x509.ParseCertificate(p.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to parse a certificate: %w", err)
	}
	return cert, nil
}

func GetPrivateKeyFromSecretKeySelector(
	ctx context.Context,
	c client.Client,
	namespace string,
	selector *corev1.SecretKeySelector,
) (*rsa.PrivateKey, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      selector.LocalObjectReference.Name,
	}
	if err := c.Get(ctx, key, &secret); err != nil {
		return nil, fmt.Errorf("unable to get Secret for a private key: %w", err)
	}

	p, _ := pem.Decode(secret.Data[corev1.TLSPrivateKeyKey])
	if p == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(p.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to parse a private key: %w", err)
	}
	return privateKey, nil
}
