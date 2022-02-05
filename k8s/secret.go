package k8s

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type SecretOption func(*corev1.Secret)

func WithType(typ corev1.SecretType) SecretOption {
	return func(secret *corev1.Secret) {
		secret.Type = typ
	}
}

func WithDataWithKey(key string, value []byte) SecretOption {
	return func(secret *corev1.Secret) {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		secret.Data[key] = value
	}
}

func ReconcileSecret(
	ctx context.Context,
	owner metav1.Object,
	scheme *runtime.Scheme,
	c client.Client,
	meta *metav1.ObjectMeta,
	opts ...func(*corev1.Secret),
) (*corev1.Secret, error) {
	var secret corev1.Secret
	meta.DeepCopyInto(&secret.ObjectMeta)
	for _, fn := range opts {
		fn(&secret)
	}
	opRes, err := ctrl.CreateOrUpdate(ctx, c, &secret, func() error {
		return ctrl.SetControllerReference(owner, &secret, scheme)
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create Secret %s: %w", ObjectName(&secret.ObjectMeta), err)
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
	}

	return &secret, nil
}

func GetValueFromSecretKeySelector(
	ctx context.Context,
	c client.Client,
	namespace string,
	selector *corev1.SecretKeySelector,
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
