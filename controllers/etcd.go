package controllers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	pointerutils "k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/errors"
	k8s_endpointslice "github.com/kkohtaka/kubernetesimal/k8s/endpointslice"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
	k8s_secret "github.com/kkohtaka/kubernetesimal/k8s/secret"
	k8s_service "github.com/kkohtaka/kubernetesimal/k8s/service"
	"github.com/kkohtaka/kubernetesimal/net/http"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
	"github.com/kkohtaka/kubernetesimal/ssh"
)

const (
	serviceNameEtcd = "etcd"
	serviceNamePeer = "peer"
	serviceNameSSH  = "ssh"

	servicePortEtcd = 2379
	servicePortPeer = 2380
	servicePortSSH  = 22

	serviceContainerPortEtcd = 2379
	serviceContainerPortPeer = 2380
	serviceContainerPortSSH  = 22
)

func newServiceName(e metav1.Object) string {
	return e.GetName()
}

func newEndpointSliceName(e metav1.Object) string {
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
		k8s_service.WithPort(serviceNameEtcd, servicePortEtcd, serviceContainerPortEtcd),
	); err != nil {
		return nil, fmt.Errorf("unable to prepare a Service for an etcd member: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: service.Name,
		}, nil
	}
}

func reconcileEndpointSlice(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	e metav1.Object,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.LocalObjectReference, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileEndpointSlice")
	defer span.End()
	logger := log.FromContext(ctx)

	var service corev1.Service
	if err := c.Get(
		ctx,
		types.NamespacedName{
			Namespace: e.GetNamespace(),
			Name:      status.ServiceRef.Name,
		},
		&service,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewRequeueError("waiting for the etcd Service prepared").Wrap(err)
		}
		return nil, err
	}

	var endpoints []discoveryv1beta1.Endpoint
	for _, ref := range status.NodeRefs {
		var (
			node    kubernetesimalv1alpha1.EtcdNode
			nodeKey = types.NamespacedName{
				Namespace: e.GetNamespace(),
				Name:      ref.Name,
			}
		)
		if err := c.Get(
			ctx,
			nodeKey,
			&node,
		); err != nil {
			if apierrors.IsNotFound(err) {
				logger.
					WithValues("etcd-node", nodeKey).
					Info("Skip appending an endpoint since EtcdNode is not found.")
				continue
			}
			return nil, err
		}

		if node.Status.PeerServiceRef == nil {
			logger.
				WithValues("etcd-node", nodeKey).
				Info("Skip appending an endpoint since EtcdNode doesn't have a Service for peer communications.")
			continue
		}
		var (
			peerService    corev1.Service
			peerServiceKey = types.NamespacedName{
				Namespace: node.Namespace,
				Name:      node.Status.PeerServiceRef.Name,
			}
		)
		if err := c.Get(
			ctx,
			peerServiceKey,
			&peerService,
		); err != nil {
			if apierrors.IsNotFound(err) {
				logger.
					WithValues("etcd-node", nodeKey).
					WithValues("service", peerServiceKey).
					Info("Skip appending an endpoint since Service is not found.")
				continue
			}
			return nil, err
		}
		if len(peerService.Spec.ClusterIPs) == 0 {
			logger.
				WithValues("etcd-node", nodeKey).
				WithValues("service", peerServiceKey).
				Info("Skip appending an endpoint since a Service doesn't have a cluster IP.")
			continue
		}

		var (
			serving     = node.Status.IsReady()
			terminating = !node.DeletionTimestamp.IsZero() || !peerService.DeletionTimestamp.IsZero()
			ready       = serving && !terminating
		)

		endpoints = append(endpoints, discoveryv1beta1.Endpoint{
			Addresses: peerService.Spec.ClusterIPs,
			Hostname:  pointerutils.StringPtr(peerService.Name),
			Conditions: discoveryv1beta1.EndpointConditions{
				Ready:       &ready,
				Serving:     &serving,
				Terminating: &terminating,
			},
			TargetRef: &corev1.ObjectReference{
				Kind:       peerService.Kind,
				Namespace:  peerService.Namespace,
				Name:       peerService.Name,
				UID:        peerService.UID,
				APIVersion: peerService.APIVersion,
			},
		})
	}

	if ep, err := k8s_endpointslice.Reconcile(
		ctx,
		e,
		c,
		newEndpointSliceName(e),
		e.GetNamespace(),
		k8s_object.WithOwner(e, scheme),
		k8s_object.WithLabel("kubernetes.io/service-name", service.Name),
		k8s_object.WithLabel("endpointslice.kubernetes.io/managed-by", "etcd-controller.kubernetesimal.kkohtaka.org"),
		k8s_endpointslice.WithAddressType(discoveryv1beta1.AddressTypeIPv4),
		k8s_endpointslice.WithPort(serviceNameEtcd, servicePortEtcd),
		k8s_endpointslice.WithEndpoints(endpoints),
	); err != nil {
		return nil, fmt.Errorf("unable to prepare an EndpointSlice for an etcd cluster: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: ep.Name,
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
		k8s_service.WithPort(serviceNameEtcd, servicePortEtcd, serviceContainerPortEtcd),
		k8s_service.WithPort(serviceNamePeer, servicePortPeer, serviceContainerPortPeer),
		k8s_service.WithPort(serviceNameSSH, servicePortSSH, serviceContainerPortSSH),
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

	var vmi kubevirtv1.VirtualMachineInstance
	if err := c.Get(
		ctx,
		types.NamespacedName{
			Namespace: en.Namespace,
			Name:      status.VirtualMachineRef.Name,
		},
		&vmi,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return errors.NewRequeueError("waiting for a VirtualMachineInstance prepared").Wrap(err)
		}
		return fmt.Errorf(
			"unable to get a VirtualMachineInstance %s/%s: %w", en.Namespace, status.VirtualMachineRef.Name, err)
	}
	if vmi.Status.Phase != kubevirtv1.Running {
		return errors.NewRequeueError("waiting for a VirtualMachineInstance become running")
	}

	privateKey, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		en.Namespace,
		spec.SSHPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return errors.NewRequeueError("waiting for an SSH private key prepared").Wrap(err)
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
			return errors.NewRequeueError("waiting for the etcd Service prepared").Wrap(err)
		}
		return err
	}
	if peerService.Spec.ClusterIP == "" {
		return errors.NewRequeueError("waiting for a cluster IP of the etcd Service prepared").
			Wrap(err).
			WithDelay(5 * time.Second)
	}
	var port int32
	for i := range peerService.Spec.Ports {
		if peerService.Spec.Ports[i].Name == serviceNameSSH {
			port = peerService.Spec.Ports[i].TargetPort.IntVal
			break
		}
	}
	if port == 0 {
		return errors.NewRequeueError("waiting for an SSH port of the etcd peer Service prepared").Wrap(err)
	}

	client, closer, err := ssh.StartSSHConnection(ctx, privateKey, peerService.Spec.ClusterIP, int(port))
	if err != nil {
		return errors.NewRequeueError("waiting for an SSH port of an etcd member prepared").
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
	ctx, span = tracing.FromContext(ctx).Start(ctx, "probeEtcdMember")
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
			return false, errors.NewRequeueError("waiting for a CA certificate prepared").Wrap(err)
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
			return false, errors.NewRequeueError("waiting for a client certificate prepared").Wrap(err)
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
			return false, errors.NewRequeueError("waiting for a client private key prepared").Wrap(err)
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

func probeEtcd(
	ctx context.Context,
	c client.Client,
	e *kubernetesimalv1alpha1.Etcd,
	spec kubernetesimalv1alpha1.EtcdSpec,
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
	address, err := k8s_service.GetAddressFromServiceRef(ctx, c, e.Namespace, "etcd", status.ServiceRef)
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
		e.Namespace,
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
		e.Namespace,
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
		e.Namespace,
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
