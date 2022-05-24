package controllers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
	k8s_secret "github.com/kkohtaka/kubernetesimal/k8s/secret"
	k8s_service "github.com/kkohtaka/kubernetesimal/k8s/service"
	"github.com/kkohtaka/kubernetesimal/net/http"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
	"github.com/kkohtaka/kubernetesimal/ssh"
)

func newServiceName(e metav1.Object) string {
	return e.GetName()
}

func newPeerServiceName(en metav1.Object) string {
	return en.GetName()
}

func reconcileService(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	e metav1.Object,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.LocalObjectReference, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileService")
	defer span.End()

	if service, err := k8s_service.Reconcile(
		ctx,
		e,
		c,
		newServiceName(e),
		e.GetNamespace(),
		k8s_object.WithOwner(e, scheme),
		k8s_service.WithType(corev1.ServiceTypeNodePort),
		k8s_service.WithPort("etcd", 2379, 2379),
		k8s_service.WithSelector("app.kubernetes.io/name", "virtualmachineimage"),
		k8s_service.WithSelector("app.kubernetes.io/part-of", "etcd"),
	); err != nil {
		return nil, fmt.Errorf("unable to prepare a Service for an etcd member: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: service.Name,
		}, nil
	}
}

func reconcilePeerService(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	en *kubernetesimalv1alpha1.EtcdNode,
	_ kubernetesimalv1alpha1.EtcdNodeSpec,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
) (*corev1.LocalObjectReference, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileService")
	defer span.End()

	if service, err := k8s_service.Reconcile(
		ctx,
		en,
		c,
		newPeerServiceName(en),
		en.Namespace,
		k8s_object.WithOwner(en, scheme),
		k8s_service.WithType(corev1.ServiceTypeNodePort),
		k8s_service.WithPort("ssh", 22, 22),
		k8s_service.WithPort("etcd", 2379, 2379),
		k8s_service.WithPort("peer", 2380, 2380),
		k8s_service.WithSelector("app.kubernetes.io/name", "virtualmachineimage"),
		k8s_service.WithSelector("app.kubernetes.io/instance", newVirtualMachineInstanceName(en)),
		k8s_service.WithSelector("app.kubernetes.io/part-of", "etcd"),
	); err != nil {
		return nil, fmt.Errorf("unable to prepare a Service for an etcd member: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: service.Name,
		}, nil
	}
}

func provisionEtcdMember(
	ctx context.Context,
	c client.Client,
	en *kubernetesimalv1alpha1.EtcdNode,
	spec kubernetesimalv1alpha1.EtcdNodeSpec,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
) error {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "provisionEtcdMember")
	defer span.End()

	privateKey, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		en.Namespace,
		spec.SSHPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return NewRequeueError("waiting for an SSH private key prepared").Wrap(err)
		}
		return err
	}

	var peerService corev1.Service
	if err := c.Get(
		ctx,
		types.NamespacedName{
			Namespace: en.Namespace,
			Name:      status.PeerServiceRef.Name,
		},
		&peerService,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return NewRequeueError("waiting for the etcd Service prepared").Wrap(err)
		}
		return err
	}
	if peerService.Spec.ClusterIP == "" {
		return NewRequeueError("waiting for a cluster IP of the etcd Service prepared").
			Wrap(err).
			WithDelay(5 * time.Second)
	}
	var port int32
	for i := range peerService.Spec.Ports {
		if peerService.Spec.Ports[i].Name == "ssh" {
			port = peerService.Spec.Ports[i].TargetPort.IntVal
			break
		}
	}
	if port == 0 {
		return NewRequeueError("waiting for an SSH port of the etcd peer Service prepared").Wrap(err)
	}

	client, closer, err := ssh.StartSSHConnection(ctx, privateKey, peerService.Spec.ClusterIP, int(port))
	if err != nil {
		return NewRequeueError("waiting for an SSH port of an etcd member prepared").
			Wrap(err).
			WithDelay(5 * time.Second)
	}
	defer closer()

	if err := ssh.RunCommandOverSSHSession(ctx, client, "sudo /opt/bin/start-etcd.sh"); err != nil {
		return err
	}

	return nil
}

func probeEtcdMember(
	ctx context.Context,
	c client.Client,
	e *kubernetesimalv1alpha1.EtcdNode,
	spec kubernetesimalv1alpha1.EtcdNodeSpec,
	status kubernetesimalv1alpha1.EtcdNodeStatus,
) (bool, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileVirtualMachineInstance")
	defer span.End()
	logger := log.FromContext(ctx)

	address, err := k8s_service.GetAddressFromServiceRef(ctx, c, e.Namespace, "etcd", status.PeerServiceRef)
	if err != nil {
		return false, fmt.Errorf("unable to get an etcd address from a peer Service: %w", err)
	}

	caCertificate, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		e.Namespace,
		spec.CACertificateRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip probing an etcd since CA certificate isn't prepared yet.")
			return false, NewRequeueError("waiting for a CA certificate prepared").Wrap(err)
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

	clientCertificate, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		e.Namespace,
		spec.ClientCertificateRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip probing an etcd since a client certificate isn't prepared yet.")
			return false, NewRequeueError("waiting for a client certificate prepared").Wrap(err)
		}
		return false, fmt.Errorf("unable to get a client certificate: %w", err)
	}

	clientPrivateKey, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		e.Namespace,
		spec.ClientPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip probing an etcd since a client private key isn't prepared yet.")
			return false, NewRequeueError("waiting for a client private key prepared").Wrap(err)
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
