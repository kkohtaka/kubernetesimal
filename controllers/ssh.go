package controllers

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
	k8s_secret "github.com/kkohtaka/kubernetesimal/k8s/secret"
	"github.com/kkohtaka/kubernetesimal/observerbility/tracing"
	"github.com/kkohtaka/kubernetesimal/ssh"
)

func newSSHKeyPairName(e *kubernetesimalv1alpha1.Etcd) string {
	return "ssh-keypair-" + e.Name
}

const (
	sshKeyPairKeyPublicKey = "ssh-publickey"
)

func reconcileSSHKeyPair(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.SecretKeySelector, *corev1.SecretKeySelector, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileSSHKeyPair")
	defer span.End()

	if status.SSHPrivateKeyRef != nil {
		if name := status.SSHPrivateKeyRef.LocalObjectReference.Name; name != newSSHKeyPairName(e) {
			return nil, nil, fmt.Errorf("invalid Secret name %s to store an SSH private key", name)
		}
	}
	if status.SSHPublicKeyRef != nil {
		if name := status.SSHPublicKeyRef.LocalObjectReference.Name; name != newSSHKeyPairName(e) {
			return nil, nil, fmt.Errorf("invalid Secret name %s to store an SSH public key", name)
		}
	}

	var sshKeyPair corev1.Secret
	if status.SSHPrivateKeyRef != nil && status.SSHPublicKeyRef != nil {
		if err := c.Get(
			ctx,
			types.NamespacedName{Namespace: e.Namespace, Name: status.SSHPrivateKeyRef.Name},
			&sshKeyPair,
		); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, nil, fmt.Errorf("unable to get a Secret for an SSH key-pair: %w", err)
			}
		} else {
			_, hasPrivateKey := sshKeyPair.Data[status.SSHPrivateKeyRef.Key]
			_, hasPublicKey := sshKeyPair.Data[status.SSHPublicKeyRef.Key]
			if hasPrivateKey && hasPublicKey {
				return status.SSHPrivateKeyRef, status.SSHPublicKeyRef, nil
			}
		}
	}

	privateKey, publicKey, err := ssh.GenerateKeyPair()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create an SSH key-pair: %w", err)
	}
	if secret, err := k8s_secret.CreateOnlyIfNotExist(
		ctx,
		e,
		c,
		newSSHKeyPairName(e),
		e.Namespace,
		k8s_object.WithOwner(e, scheme),
		k8s_secret.WithType(corev1.SecretTypeSSHAuth),
		k8s_secret.WithDataWithKey(corev1.SSHAuthPrivateKey, privateKey),
		k8s_secret.WithDataWithKey(sshKeyPairKeyPublicKey, publicKey),
	); err != nil {
		return nil, nil, fmt.Errorf("unable to prepare a Secret for an SSH key-pair: %w", err)
	} else {
		return &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: corev1.SSHAuthPrivateKey,
			},
			&corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: sshKeyPairKeyPublicKey,
			},
			nil
	}
}

func finalizeSSHKeyPairSecret(
	ctx context.Context,
	c client.Client,
	e *kubernetesimalv1alpha1.Etcd,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizeSSHKeyPairSecret")
	defer span.End()

	if status.SSHPrivateKeyRef == nil {
		return status, nil
	}
	if err := finalizeSecret(ctx, c, e.Namespace, status.SSHPrivateKeyRef.Name); err != nil {
		return status, err
	}
	status.SSHPrivateKeyRef = nil
	status.SSHPublicKeyRef = nil
	log.FromContext(ctx).Info("SSH key-pair was finalized.")
	return status, nil
}
