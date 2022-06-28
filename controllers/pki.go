package controllers

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/errors"
	"github.com/kkohtaka/kubernetesimal/controller/finalizer"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
	k8s_secret "github.com/kkohtaka/kubernetesimal/k8s/secret"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
	"github.com/kkohtaka/kubernetesimal/pki"
)

func newCACertificateName(e metav1.Object) string {
	return "ca-" + e.GetName()
}

func newCACertificateIssuerName(e metav1.Object) string {
	return e.GetName()
}

func reconcileCACertificate(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	e metav1.Object,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.SecretKeySelector, *corev1.SecretKeySelector, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileCACertificate")
	defer span.End()

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
		if err := c.Get(
			ctx,
			types.NamespacedName{Namespace: e.GetNamespace(), Name: status.CAPrivateKeyRef.Name},
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
	if secret, err := k8s_secret.CreateOnlyIfNotExist(
		ctx,
		e,
		c,
		newCACertificateName(e),
		e.GetNamespace(),
		k8s_object.WithOwner(e, scheme),
		k8s_secret.WithType(corev1.SecretTypeTLS),
		k8s_secret.WithDataWithKey(corev1.TLSCertKey, certificate),
		k8s_secret.WithDataWithKey(corev1.TLSPrivateKeyKey, privateKey),
	); err != nil {
		return nil, nil, fmt.Errorf("unable to prepare a Secret for a CA certificate for etcd: %w", err)
	} else {
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

func finalizeCACertificateSecret(
	ctx context.Context,
	c client.Client,
	e metav1.Object,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizeCACertificateSecret")
	defer span.End()

	if status.CACertificateRef == nil {
		return status, nil
	}
	if err := finalizer.FinalizeSecret(ctx, c, e.GetNamespace(), status.CACertificateRef.Name); err != nil {
		return status, err
	}
	status.CACertificateRef = nil
	log.FromContext(ctx).Info("CA certificate was finalized.")
	return status, nil
}

func newClientCertificateName(e metav1.Object) string {
	return "api-client-" + e.GetName()
}

func newPeerCertificateName(e metav1.Object) string {
	return "peer-" + e.GetName()
}

func reconcileClientCertificate(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	e metav1.Object,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.SecretKeySelector, *corev1.SecretKeySelector, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileClientCertificate")
	defer span.End()

	if status.ClientPrivateKeyRef != nil {
		if name := status.ClientPrivateKeyRef.LocalObjectReference.Name; name != newClientCertificateName(e) {
			return nil, nil, fmt.Errorf("invalid Secret name %s to store a client private key", name)
		}
	}
	if status.ClientCertificateRef != nil {
		if name := status.ClientCertificateRef.LocalObjectReference.Name; name != newClientCertificateName(e) {
			return nil, nil, fmt.Errorf("invalid Secret name %s to store a client certificate", name)
		}
	}

	var secret corev1.Secret
	if status.ClientPrivateKeyRef != nil && status.ClientCertificateRef != nil {
		if err := c.Get(
			ctx,
			types.NamespacedName{Namespace: e.GetNamespace(), Name: status.ClientPrivateKeyRef.Name},
			&secret,
		); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, nil, fmt.Errorf("unable to get a Secret for a client certificate: %w", err)
			}
		} else {
			_, hasPublicKey := secret.Data[status.ClientCertificateRef.Key]
			_, hasPrivateKey := secret.Data[status.ClientPrivateKeyRef.Key]
			if hasPublicKey && hasPrivateKey {
				return status.ClientCertificateRef, status.ClientPrivateKeyRef, nil
			}
		}
	}

	caCert, err := k8s_secret.GetCertificateFromSecretKeySelector(
		ctx,
		c,
		e.GetNamespace(),
		status.CACertificateRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, errors.NewRequeueError("waiting for a CA certificate prepared")
		}
		return nil, nil, fmt.Errorf("unable to load a CA certificate from a Secret: %w", err)
	}

	caPrivateKey, err := k8s_secret.GetPrivateKeyFromSecretKeySelector(
		ctx,
		c,
		e.GetNamespace(),
		status.CAPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, errors.NewRequeueError("waiting for a CA private key prepared")
		}
		return nil, nil, fmt.Errorf("unable to load a CA private key from a Secret: %w", err)
	}

	certificate, privateKey, err := pki.CreateClientCertificateAndPrivateKey(
		newClientCertificateName(e),
		caCert,
		caPrivateKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create a client certificate for etcd: %w", err)
	}
	if secret, err := k8s_secret.CreateOnlyIfNotExist(
		ctx,
		e,
		c,
		newClientCertificateName(e),
		e.GetNamespace(),
		k8s_object.WithOwner(e, scheme),
		k8s_secret.WithType(corev1.SecretTypeTLS),
		k8s_secret.WithDataWithKey(corev1.TLSCertKey, certificate),
		k8s_secret.WithDataWithKey(corev1.TLSPrivateKeyKey, privateKey),
	); err != nil {
		return nil, nil, fmt.Errorf("unable to prepare a Secret for a client certificate for etcd: %w", err)
	} else {
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

func reconcilePeerCertificate(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	e metav1.Object,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.SecretKeySelector, *corev1.SecretKeySelector, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcilePeerCertificate")
	defer span.End()

	if status.PeerPrivateKeyRef != nil {
		if name := status.PeerPrivateKeyRef.LocalObjectReference.Name; name != newPeerCertificateName(e) {
			return nil, nil, fmt.Errorf("invalid Secret name %s to store a private key for peer communication", name)
		}
	}
	if status.PeerCertificateRef != nil {
		if name := status.PeerCertificateRef.LocalObjectReference.Name; name != newPeerCertificateName(e) {
			return nil, nil, fmt.Errorf("invalid Secret name %s to store a certificate for peer communication", name)
		}
	}

	var secret corev1.Secret
	if status.PeerPrivateKeyRef != nil && status.PeerCertificateRef != nil {
		if err := c.Get(
			ctx,
			types.NamespacedName{Namespace: e.GetNamespace(), Name: status.PeerPrivateKeyRef.Name},
			&secret,
		); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, nil, fmt.Errorf("unable to get a Secret for a certificate for peer communication: %w", err)
			}
		} else {
			_, hasPublicKey := secret.Data[status.PeerCertificateRef.Key]
			_, hasPrivateKey := secret.Data[status.PeerPrivateKeyRef.Key]
			if hasPublicKey && hasPrivateKey {
				return status.PeerCertificateRef, status.PeerPrivateKeyRef, nil
			}
		}
	}

	caCert, err := k8s_secret.GetCertificateFromSecretKeySelector(
		ctx,
		c,
		e.GetNamespace(),
		status.CACertificateRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, errors.NewRequeueError("waiting for a CA certificate prepared")
		}
		return nil, nil, fmt.Errorf("unable to load a CA certificate from a Secret: %w", err)
	}

	caPrivateKey, err := k8s_secret.GetPrivateKeyFromSecretKeySelector(
		ctx,
		c,
		e.GetNamespace(),
		status.CAPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, errors.NewRequeueError("waiting for a CA private key prepared")
		}
		return nil, nil, fmt.Errorf("unable to load a CA private key from a Secret: %w", err)
	}

	certificate, privateKey, err := pki.CreateClientCertificateAndPrivateKey(
		newPeerCertificateName(e),
		caCert,
		caPrivateKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create a certificate for etcd peer communication: %w", err)
	}
	if secret, err := k8s_secret.CreateOnlyIfNotExist(
		ctx,
		e,
		c,
		newPeerCertificateName(e),
		e.GetNamespace(),
		k8s_object.WithOwner(e, scheme),
		k8s_secret.WithType(corev1.SecretTypeTLS),
		k8s_secret.WithDataWithKey(corev1.TLSCertKey, certificate),
		k8s_secret.WithDataWithKey(corev1.TLSPrivateKeyKey, privateKey),
	); err != nil {
		return nil, nil, fmt.Errorf("unable to prepare a Secret for a certificate for etcd peer communication: %w", err)
	} else {
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

func finalizeClientCertificateSecret(
	ctx context.Context,
	c client.Client,
	e metav1.Object,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizeClientCertificateSecret")
	defer span.End()

	if status.ClientCertificateRef == nil {
		return status, nil
	}
	if err := finalizer.FinalizeSecret(ctx, c, e.GetNamespace(), status.ClientCertificateRef.Name); err != nil {
		return status, err
	}
	status.ClientCertificateRef = nil
	log.FromContext(ctx).Info("Client certificate was finalized.")
	return status, nil
}

func finalizePeerCertificateSecret(
	ctx context.Context,
	c client.Client,
	e metav1.Object,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizePeerCertificateSecret")
	defer span.End()

	if status.PeerCertificateRef == nil {
		return status, nil
	}
	if err := finalizer.FinalizeSecret(ctx, c, e.GetNamespace(), status.PeerCertificateRef.Name); err != nil {
		return status, err
	}
	status.PeerCertificateRef = nil
	log.FromContext(ctx).Info("Client certificate was finalized.")
	return status, nil
}
