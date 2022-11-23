package etcd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sort"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
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

const (
	defaultRequestTimeout = 5 * time.Second

	defaultMemberStatusTimeout = time.Second
)

func getEtcdTLSConfig(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	status *kubernetesimalv1alpha1.EtcdStatus,
) (*tls.Config, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "getEtcdTLSConfig")
	defer span.End()
	logger := log.FromContext(ctx)

	if status.CACertificateRef == nil {
		logger.V(4).Info("a CA certificate for an etcd Service is not prepared yet")
		return nil, nil
	}
	caCertificate, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
		*status.CACertificateRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(4).Info("Skip probing an etcd since CA certificate isn't prepared yet.")
			return nil, nil
		}
		return nil, fmt.Errorf("unable to get a CA certificate: %w", err)
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("unable to load a client CA certificates from the system: %w", err)
	}
	if ok := rootCAs.AppendCertsFromPEM(caCertificate); !ok {
		return nil, fmt.Errorf("unable to load a client CA certificate from Secret")
	}

	if status.ClientCertificateRef == nil {
		return nil, fmt.Errorf("a client certificate for an etcd Service is not prepared yet")
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
			return nil, nil
		}
		return nil, fmt.Errorf("unable to get a client certificate: %w", err)
	}

	if status.ClientCertificateRef == nil {
		return nil, fmt.Errorf("a client certificate for an etcd Service is not prepared yet")
	}
	clientPrivateKey, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
		*status.ClientPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(4).Info("Skip probing an etcd since a client private key isn't prepared yet.")
			return nil, nil
		}
		return nil, fmt.Errorf("unable to get a client private key: %w", err)
	}

	certificate, err := tls.X509KeyPair(clientCertificate, clientPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("unable to load a client certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{
			certificate,
		},
		RootCAs:            rootCAs,
		InsecureSkipVerify: true,
	}, nil
}

func probeEtcd(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	_ *kubernetesimalv1alpha1.EtcdSpec,
	status *kubernetesimalv1alpha1.EtcdStatus,
) (bool, string, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "probeEtcd")
	defer span.End()
	logger := log.FromContext(ctx)

	if status.ServiceRef == nil {
		logger.V(4).Info("a Service for an etcd is not prepared yet")
		return false, "a Service is not prepared yet", nil
	}
	address, err := k8s_service.GetAddressFromServiceRef(ctx, c, obj.GetNamespace(), "etcd", status.ServiceRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(4).Info("Skip probing an etcd since an etcd Service isn't prepared yet.")
			return false, "a Service is not prepared yet", nil
		}
		return false, "", fmt.Errorf("unable to get an etcd address from an etcd Service: %w", err)
	}

	tlsConfig, err := getEtcdTLSConfig(ctx, c, obj, status)
	if err != nil {
		return false, "", fmt.Errorf("unable to get a TLS config for an etcd cluster: %w", err)
	}

	probed, err := http.NewProber(
		fmt.Sprintf("https://%s/health", address),
		http.WithTLSConfig(tlsConfig),
	).Once(ctx)
	if err != nil {
		return false, "", err
	}
	return probed, "", nil
}

func probeEtcdMembers(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	_ *kubernetesimalv1alpha1.EtcdSpec,
	status *kubernetesimalv1alpha1.EtcdStatus,
) (bool, string, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "probeEtcdMembers")
	defer span.End()
	logger := log.FromContext(ctx)

	if status.ServiceRef == nil {
		logger.V(4).Info("a Service for an etcd is not prepared yet")
		return false, "a Service is not prepared yet", nil
	}
	address, err := k8s_service.GetAddressFromServiceRef(ctx, c, obj.GetNamespace(), "etcd", status.ServiceRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(4).Info("Skip probing an etcd since an etcd Service isn't prepared yet.")
			return false, "a Service is not prepared yet", nil
		}
		return false, "", fmt.Errorf("unable to get an etcd address from an etcd Service: %w", err)
	}

	tlsConfig, err := getEtcdTLSConfig(ctx, c, obj, status)
	if err != nil {
		return false, "", fmt.Errorf("unable to get a TLS config for an etcd cluster: %w", err)
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints: []string{
			fmt.Sprintf("https://%s", address),
		},
		TLS: tlsConfig,
	})
	if err != nil {
		return false, "", fmt.Errorf("unable to create an etcd client: %w", err)
	}

	listMemberCtx, listMemberCancel := context.WithTimeout(ctx, defaultRequestTimeout)
	resp, err := client.MemberList(listMemberCtx)
	listMemberCancel()
	if err != nil {
		return false, "", fmt.Errorf("unable to list etcd members: %w", err)
	}
	logger.V(4).Info("List etcd members.", "members", resp.Members)

	nodes, err := getComponentEtcdNodes(ctx, c, obj)
	if err != nil {
		return false, "", fmt.Errorf("unable to list component EtcdNodes: %w", err)
	}

	probed := map[string]bool{}
	for _, node := range nodes {
		probed[node.GetName()] = false
	}

members:
	for _, member := range resp.Members {
		if _, ok := probed[member.Name]; !ok {
			return false, fmt.Sprintf("a member %q is not listed", member.Name), nil
		}

		for _, url := range member.GetClientURLs() {
			if ok := func(u string) bool {
				c, err := clientv3.New(clientv3.Config{
					Endpoints: []string{u},
					TLS:       tlsConfig,
				})
				if err != nil {
					logger.Error(err, "Creating an etcd client to check member's health was failed.")
					return false
				}

				statusCtx, statusCancel := context.WithTimeout(ctx, defaultMemberStatusTimeout)
				defer statusCancel()
				if _, err := c.Status(statusCtx, u); err != nil {
					logger.V(4).Error(err, "Checking a status of an etcd member was failed.")
					return false
				}
				return true
			}(url); ok {
				logger.V(4).Info("Succeeded in probing an URL", "url", url)
				probed[member.Name] = true
				continue members
			}
			logger.V(4).Info("Failed probing an URL", "url", url)
		}
		return false, fmt.Sprintf("a member %q was not probed", member.Name), nil
	}

	var notFoundNodes []string
	for nodeName, ok := range probed {
		if !ok {
			notFoundNodes = append(notFoundNodes, fmt.Sprintf("%q", nodeName))
		}
	}
	if len(notFoundNodes) > 0 {
		sort.Strings(notFoundNodes)
		return false, fmt.Sprintf("[%s] are not members", strings.Join(notFoundNodes, ", ")), nil
	}
	return true, "", nil
}
