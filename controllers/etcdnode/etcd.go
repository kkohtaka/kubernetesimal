package etcdnode

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/errors"
	k8s_secret "github.com/kkohtaka/kubernetesimal/k8s/secret"
	k8s_service "github.com/kkohtaka/kubernetesimal/k8s/service"
	"github.com/kkohtaka/kubernetesimal/net/http"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
	"github.com/kkohtaka/kubernetesimal/ssh"
)

func provisionEtcdMember(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeSpec,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) error {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "provisionEtcdMember")
	defer span.End()

	var vmi kubevirtv1.VirtualMachineInstance
	if err := c.Get(
		ctx,
		types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      status.VirtualMachineRef.Name,
		},
		&vmi,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return errors.NewRequeueError("waiting for a VirtualMachineInstance prepared").Wrap(err)
		}
		return fmt.Errorf(
			"unable to get a VirtualMachineInstance %s/%s: %w", obj.GetNamespace(), status.VirtualMachineRef.Name, err)
	}
	if vmi.Status.Phase != kubevirtv1.Running {
		return errors.NewRequeueError("waiting for a VirtualMachineInstance become running")
	}

	privateKey, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
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
			Namespace: obj.GetNamespace(),
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

	if spec.AsFirstNode {
		if err := ssh.RunCommandOverSSHSession(ctx, client, "sudo /opt/bin/start-cluster.sh"); err != nil {
			return err
		}
	} else {
		if err := ssh.RunCommandOverSSHSession(ctx, client, "sudo /opt/bin/join-cluster.sh"); err != nil {
			return err
		}
	}

	return nil
}

func probeEtcdMember(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeSpec,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) (bool, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "probeEtcdMember")
	defer span.End()
	logger := log.FromContext(ctx)

	address, err := k8s_service.GetAddressFromServiceRef(ctx, c, obj.GetNamespace(), "etcd", status.PeerServiceRef)
	if err != nil {
		return false, fmt.Errorf("unable to get an etcd address from a peer Service: %w", err)
	}

	caCertificate, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
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
		obj.GetNamespace(),
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
		obj.GetNamespace(),
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

func finalizeEtcdMember(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeSpec,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) (*kubernetesimalv1alpha1.EtcdNodeStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizeEtcdMember")
	defer span.End()
	logger := log.FromContext(ctx)

	if status.IsMemberFinalized() {
		logger.V(4).Info("An etcd member is already finalized.")
		return status, nil
	}

	if !status.IsProvisioned() {
		logger.V(4).Info("Skip finalizing an etcd member since an etcd member was not provisioned")
		return status, nil
	}

	if status.VirtualMachineRef == nil {
		logger.V(4).Info("Skip finalizing an etcd member since a VirtualMachine doesn't exit")
		return status, nil
	}

	var vmi kubevirtv1.VirtualMachineInstance
	if err := c.Get(
		ctx,
		types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      status.VirtualMachineRef.Name,
		},
		&vmi,
	); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(4).Info("Skip finalizing an etcd member since a VirtualMachine doesn't exit")
			return status, nil
		}
		return status, fmt.Errorf(
			"unable to get a VirtualMachineInstance %s/%s: %w", obj.GetNamespace(), status.VirtualMachineRef.Name, err)
	}

	privateKey, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
		spec.SSHPrivateKeyRef,
	)
	if err != nil {
		return status, fmt.Errorf(
			"unable to get an SSH private key %s/%s: %w", obj.GetNamespace(), spec.SSHPrivateKeyRef.Name, err)
	}

	var peerService corev1.Service
	if err := c.Get(
		ctx,
		types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      status.PeerServiceRef.Name,
		},
		&peerService,
	); err != nil {
		return status, fmt.Errorf(
			"unable to get the etcd Service %s/%s: %w", obj.GetNamespace(), status.PeerServiceRef.Name, err)
	}
	var port int32
	for i := range peerService.Spec.Ports {
		if peerService.Spec.Ports[i].Name == serviceNameSSH {
			port = peerService.Spec.Ports[i].TargetPort.IntVal
			break
		}
	}

	client, closer, err := ssh.StartSSHConnection(ctx, privateKey, peerService.Spec.ClusterIP, int(port))
	if err != nil {
		err = errors.NewRequeueError("waiting for an SSH port of an etcd member prepared").
			Wrap(err).
			WithDelay(5 * time.Second)
		return status.WithMemberFinalized(false, err.Error()), err
	}
	defer closer()

	if err := ssh.RunCommandOverSSHSession(ctx, client, "sudo /opt/bin/leave-cluster.sh"); err != nil {
		return status.WithMemberFinalized(false, err.Error()), err
	}
	logger.Info("An etcd member was finalized successfully.")
	return status.WithMemberFinalized(true, ""), nil
}
