package etcd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"go.opentelemetry.io/otel/trace"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	k8s_secret "github.com/kkohtaka/kubernetesimal/k8s/secret"
	k8s_service "github.com/kkohtaka/kubernetesimal/k8s/service"
	"github.com/kkohtaka/kubernetesimal/net/http"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

func probeEtcd(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	_ *kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (bool, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "probeEtcd")
	defer span.End()
	logger := log.FromContext(ctx)

	if status.ServiceRef == nil {
		logger.Info("a Service for an etcd is not prepared yet")
		return false, nil
	}
	address, err := k8s_service.GetAddressFromServiceRef(ctx, c, obj.GetNamespace(), "etcd", status.ServiceRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip probing an etcd since an etcd Service isn't prepared yet.")
			return false, nil
		}
		return false, fmt.Errorf("unable to get an etcd address from an etcd Service: %w", err)
	}

	if status.CACertificateRef == nil {
		logger.Info("a CA certificate for an etcd Service is not prepared yet")
		return false, nil
	}
	caCertificate, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
		*status.CACertificateRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip probing an etcd since CA certificate isn't prepared yet.")
			return false, nil
		}
		return false, fmt.Errorf("unable to get a CA certificate: %w", err)
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return false, fmt.Errorf("unable to load a client CA certificates from the system: %w", err)
	}
	if ok := rootCAs.AppendCertsFromPEM(caCertificate); !ok {
		return false, fmt.Errorf("unable to load a client CA certificate from Secret")
	}

	if status.ClientCertificateRef == nil {
		return false, fmt.Errorf("a client certificate for an etcd Service is not prepared yet")
	}
	clientCertificate, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
		*status.ClientCertificateRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip probing an etcd since a client certificate isn't prepared yet.")
			return false, nil
		}
		return false, fmt.Errorf("unable to get a client certificate: %w", err)
	}

	if status.ClientCertificateRef == nil {
		return false, fmt.Errorf("a client certificate for an etcd Service is not prepared yet")
	}
	clientPrivateKey, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
		*status.ClientPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip probing an etcd since a client private key isn't prepared yet.")
			return false, nil
		}
		return false, fmt.Errorf("unable to get a client private key: %w", err)
	}

	certificate, err := tls.X509KeyPair(clientCertificate, clientPrivateKey)
	if err != nil {
		return false, fmt.Errorf("unable to load a client certificate: %w", err)
	}

	return http.NewProber(
		fmt.Sprintf("https://%s/health", address),
		http.WithTLSConfig(&tls.Config{
			Certificates: []tls.Certificate{
				certificate,
			},
			RootCAs:            rootCAs,
			InsecureSkipVerify: true,
		}),
	).Once(ctx)
}
