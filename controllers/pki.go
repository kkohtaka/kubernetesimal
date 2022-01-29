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
	"github.com/kkohtaka/kubernetesimal/pki"
)

func newCACertificateName(e *kubernetesimalv1alpha1.Etcd) string {
	return "ca-" + e.Name
}

func newCACertificateIssuerName(e *kubernetesimalv1alpha1.Etcd) string {
	return e.Name
}

func (r *EtcdReconciler) reconcileCACertificate(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.SecretKeySelector, *corev1.SecretKeySelector, error) {
	logger := log.FromContext(ctx)

	if status.CAPrivateKeyRef != nil {
		if name := status.CAPrivateKeyRef.LocalObjectReference.Name; name != newCACertificateName(e) {
			return nil, nil, fmt.Errorf("invalid Secret name %s to store a CA private key", name)
		}
	}
	if status.CACertificateRef != nil {
		if name := status.CACertificateRef.LocalObjectReference.Name; name != newCACertificateName(e) {
			return nil, nil, fmt.Errorf("invalid Secret name %s to store a CA certificate", name)
		}
	}

	var ca corev1.Secret
	if status.CAPrivateKeyRef != nil && status.CACertificateRef != nil {
		if err := r.Client.Get(
			ctx,
			types.NamespacedName{Namespace: e.Namespace, Name: status.CAPrivateKeyRef.Name},
			&ca,
		); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, nil, fmt.Errorf("unable to get a Secret for a CA certificate: %w", err)
			}
		} else {
			_, hasPublicKey := ca.Data[status.CACertificateRef.Key]
			_, hasPrivateKey := ca.Data[status.CAPrivateKeyRef.Key]
			if hasPublicKey && hasPrivateKey {
				return status.CACertificateRef, status.CAPrivateKeyRef, nil
			}
		}
	}

	certificate, privateKey, err := pki.CreateCACertificateAndPrivateKey(newCACertificateIssuerName(e))
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create a CA certificate for etcd: %w", err)
	}
	if secret, err := k8s.ReconcileSecret(
		ctx,
		e,
		r.Scheme,
		r.Client,
		k8s.NewObjectMeta(
			k8s.WithName(newCACertificateName(e)),
			k8s.WithNamespace(e.Namespace),
		),
		k8s.WithType(corev1.SecretTypeTLS),
		k8s.WithDataWithKey(corev1.TLSCertKey, certificate),
		k8s.WithDataWithKey(corev1.TLSPrivateKeyKey, privateKey),
	); err != nil {
		return nil, nil, fmt.Errorf("unable to prepare a Secret for a CA certificate for etcd: %w", err)
	} else {
		logger.Info("A Secret for CA certificate was prepared.")
		return &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: corev1.TLSCertKey,
			},
			&corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: corev1.TLSPrivateKeyKey,
			},
			nil
	}
}

func (r *EtcdReconciler) finalizeCACertificateSecret(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, bool, error) {
	logger := log.FromContext(ctx)

	if e.Status.CACertificateRef == nil {
		return status, true, nil
	}
	if deleted, err := r.finalizeSecret(ctx, e.Namespace, e.Status.CACertificateRef.Name); err != nil {
		return status, false, err
	} else if !deleted {
		return status, false, nil
	}
	logger.Info("CA certificate was finalized.")
	status.CACertificateRef = nil
	return status, true, nil
}
