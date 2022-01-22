package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	_, err := ctrl.CreateOrUpdate(ctx, c, &secret, func() error {
		return ctrl.SetControllerReference(owner, &secret, scheme)
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create Secret %s: %w", ObjectName(&secret.ObjectMeta), err)
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
