package controllers

import (
	"context"
	"crypto/tls"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/k8s"
	k8s_service "github.com/kkohtaka/kubernetesimal/k8s/service"
	"github.com/kkohtaka/kubernetesimal/net/http"
	"github.com/kkohtaka/kubernetesimal/ssh"
)

func newServiceName(e *kubernetesimalv1alpha1.Etcd) string {
	return e.Name
}

func (r *EtcdReconciler) reconcileService(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.LocalObjectReference, error) {
	logger := log.FromContext(ctx)

	if service, err := k8s_service.Reconcile(
		ctx,
		e,
		r.Scheme,
		r.Client,
		k8s.NewObjectMeta(
			k8s.WithName(newServiceName(e)),
			k8s.WithNamespace(e.Namespace),
		),
		k8s_service.WithType(corev1.ServiceTypeNodePort),
		k8s_service.WithPort("ssh", 22, 22),
		k8s_service.WithPort("etcd", 2379, 2379),
		k8s_service.WithSelector("app.kubernetes.io/name", "virtualmachineimage"),
		k8s_service.WithSelector("app.kubernetes.io/instance", newVirtualMachineInstanceName(e)),
		k8s_service.WithSelector("app.kubernetes.io/part-of", "etcd"),
	); err != nil {
		return nil, fmt.Errorf("unable to prepare a Service for an etcd member: %w", err)
	} else {
		logger.Info("A Service for an etcd member was prepared.")
		return &corev1.LocalObjectReference{
			Name: service.Name,
		}, nil
	}
}

func (r *EtcdReconciler) provisionEtcdMember(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (bool, error) {
	logger := log.FromContext(ctx)

	privateKey, err := k8s.GetValueFromSecretKeySelector(
		ctx,
		r.Client,
		e.Namespace,
		status.SSHPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip provisioning an etcd member since SSH private key isn't prepared yet.")
			return false, nil
		}
		return false, err
	}

	var service corev1.Service
	if err := r.Get(
		ctx,
		types.NamespacedName{
			Namespace: e.Namespace,
			Name:      status.ServiceRef.Name,
		},
		&service,
	); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip provisioning an etcd member since the etcd Service isn't prepared yet.")
			return false, nil
		}
		return false, err
	}
	if service.Spec.ClusterIP == "" {
		logger.Info("Skip provisioning an etcd member since cluster ip isn't assigned yet.")
		return false, nil
	}
	var port int32
	for i := range service.Spec.Ports {
		if service.Spec.Ports[i].Name == "ssh" {
			port = service.Spec.Ports[i].TargetPort.IntVal
			break
		}
	}
	if port == 0 {
		logger.Info("Skip provisioning an etcd member since port of service %s/%s isn't assigned yet.")
		return false, nil
	}

	client, closer, err := ssh.StartSSHConnection(ctx, privateKey, service.Spec.ClusterIP, int(port))
	if err != nil {
		logger.Info(
			"Skip provisioning an etcd member since SSH port of an etcd member isn't available yet.",
			"reason", err,
		)
		return false, nil
	}
	defer closer()

	if err := ssh.RunCommandOverSSHSession(ctx, client, "sudo /opt/bin/start-etcd.sh"); err != nil {
		return false, err
	}
	logger.Info("Succeeded in executing a start-up script for an etcd member on the VirtualMachineInstance.")

	return true, nil
}

func (r *EtcdReconciler) probeEtcdMember(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (bool, error) {
	address, err := k8s_service.GetAddressFromServiceRef(ctx, r.Client, e.Namespace, "etcd", status.ServiceRef)
	if err != nil {
		return false, fmt.Errorf("unable to get an etcd address from a Service: %w", err)
	}
	return http.NewProber(
		fmt.Sprintf("https://%s/health", address),
		http.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	).Once(ctx)
}
