package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/k8s"
	"github.com/kkohtaka/kubernetesimal/ssh"
)

func newSSHKeyPairName(e *kubernetesimalv1alpha1.Etcd) string {
	return "ssh-keypair-" + e.Name
}

const (
	sshKeyPairKeyPublicKey = "ssh-publickey"
)

func (r *EtcdReconciler) reconcileSSHKeyPair(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.SecretKeySelector, *corev1.SecretKeySelector, error) {
	logger := log.FromContext(ctx)

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
		if err := r.Client.Get(
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
	if secret, err := k8s.ReconcileSecret(
		ctx,
		e,
		r.Scheme,
		r.Client,
		k8s.NewObjectMeta(
			k8s.WithName(newSSHKeyPairName(e)),
			k8s.WithNamespace(e.Namespace),
		),
		k8s.WithType(corev1.SecretTypeSSHAuth),
		k8s.WithDataWithKey(corev1.SSHAuthPrivateKey, privateKey),
		k8s.WithDataWithKey(sshKeyPairKeyPublicKey, publicKey),
	); err != nil {
		return nil, nil, fmt.Errorf("unable to prepare a Secret for an SSH key-pair: %w", err)
	} else {
		logger.Info("A Secret for an SSH key-pair was prepared.")
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

func (r *EtcdReconciler) finalizeSSHKeyPairSecret(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, bool, error) {
	logger := log.FromContext(ctx)

	if e.Status.SSHPrivateKeyRef == nil {
		return status, true, nil
	}
	if deleted, err := r.finalizeSecret(ctx, e.Namespace, e.Status.SSHPrivateKeyRef.Name); err != nil {
		return status, false, err
	} else if !deleted {
		return status, false, nil
	}
	logger.Info("SSH key-pair was finalized.")
	status.SSHPrivateKeyRef = nil
	status.SSHPublicKeyRef = nil
	return status, true, nil
}