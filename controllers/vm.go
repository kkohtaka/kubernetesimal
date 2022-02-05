package controllers

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/k8s"
)

var (
	//go:embed templates/*.tmpl
	cloudConfigTemplates embed.FS
)

const (
	defaultEtcdadmReleaseURL = "https://github.com/kubernetes-sigs/etcdadm/releases/download"
)

var (
	defaultEtcdadmVersion = "0.1.5"

	defaultEtcdVersion = "3.5.1"
)

func newUserDataName(e *kubernetesimalv1alpha1.Etcd) string {
	return "userdata-" + e.Name
}

func newVirtualMachineInstanceName(e *kubernetesimalv1alpha1.Etcd) string {
	return e.Name
}

func (r *EtcdReconciler) reconcileUserData(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.LocalObjectReference, error) {
	publicKey, err := k8s.GetValueFromSecretKeySelector(
		ctx,
		r.Client,
		e.Namespace,
		status.SSHPublicKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, NewRequeueError("waiting for an SSH public key prepared").Wrap(err)
		}
		return nil, fmt.Errorf("unable to get an SSH public key: %w", err)
	}

	caCertificate, err := k8s.GetValueFromSecretKeySelector(
		ctx,
		r.Client,
		e.Namespace,
		status.CACertificateRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, NewRequeueError("waiting for a CA certificate prepared").Wrap(err)
		}
		return nil, fmt.Errorf("unable to get a CA certificate: %w", err)
	}

	caPrivateKey, err := k8s.GetValueFromSecretKeySelector(
		ctx,
		r.Client,
		e.Namespace,
		status.CAPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, NewRequeueError("waiting for a CA private key prepared").Wrap(err)
		}
		return nil, fmt.Errorf("unable to get a CA private key: %w", err)
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
			return nil, NewRequeueError("waiting for the etcd Service prepared").Wrap(err)
		}
		return nil, fmt.Errorf("unable to get a service %s/%s: %w", e.Namespace, status.ServiceRef.Name, err)
	}
	if service.Spec.ClusterIP == "" {
		return nil, NewRequeueError("waiting for a cluster IP of the etcd Service prepared").Wrap(err)
	}

	startEtcdScriptBuf := bytes.Buffer{}
	startEtcdScriptTmpl, err := template.New("start-etcd.sh.tmpl").ParseFS(cloudConfigTemplates, "templates/start-etcd.sh.tmpl")
	if err != nil {
		return nil, fmt.Errorf("unable to parse a template of start-etcd.sh: %w", err)
	}
	if err := startEtcdScriptTmpl.Execute(
		&startEtcdScriptBuf,
		&struct {
			EtcdadmReleaseURL string
			EtcdadmVersion    string
			EtcdVersion       string
			ServiceIP         string
			ServiceName       string
			ServiceNamespace  string
		}{
			EtcdadmReleaseURL: defaultEtcdadmReleaseURL,
			EtcdadmVersion:    defaultEtcdadmVersion,
			EtcdVersion:       defaultEtcdVersion,
			ServiceIP:         service.Spec.ClusterIP,
			ServiceName:       service.Name,
			ServiceNamespace:  service.Namespace,
		},
	); err != nil {
		return nil, fmt.Errorf("unable to render start-etcd.sh from a template: %w", err)
	}

	cloudInitBuf := bytes.Buffer{}
	cloudInitTmpl, err := template.New("cloud-init.tmpl").ParseFS(cloudConfigTemplates, "templates/cloud-init.tmpl")
	if err != nil {
		return nil, fmt.Errorf("unable to parse a template of cloud-init: %w", err)
	}
	if err := cloudInitTmpl.Execute(
		&cloudInitBuf,
		&struct {
			AuthorizedKeys  []string
			StartEtcdScript string
			CACertificate   string
			CAPrivateKey    string
		}{
			AuthorizedKeys:  []string{string(publicKey)},
			StartEtcdScript: base64.StdEncoding.EncodeToString(startEtcdScriptBuf.Bytes()),
			CACertificate:   base64.StdEncoding.EncodeToString(caCertificate),
			CAPrivateKey:    base64.StdEncoding.EncodeToString(caPrivateKey),
		},
	); err != nil {
		return nil, fmt.Errorf("unable to render a cloud-config from a template: %w", err)
	}

	if secret, err := k8s.ReconcileSecret(
		ctx,
		e,
		r.Scheme,
		r.Client,
		k8s.NewObjectMeta(
			k8s.WithName(newUserDataName(e)),
			k8s.WithNamespace(e.Namespace),
		),
		k8s.WithDataWithKey("userdata", cloudInitBuf.Bytes()),
	); err != nil {
		return nil, fmt.Errorf("unable to create Secret: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: secret.Name,
		}, nil
	}
}

func (r *EtcdReconciler) reconcileVirtualMachineInstance(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.LocalObjectReference, error) {
	if vmi, err := k8s.ReconcileVirtualMachineInstance(
		ctx,
		e,
		r.Scheme,
		r.Client,
		k8s.NewObjectMeta(
			k8s.WithName(newVirtualMachineInstanceName(e)),
			k8s.WithNamespace(e.Namespace),
			k8s.WithLabel("app.kubernetes.io/name", "virtualmachineimage"),
			k8s.WithLabel("app.kubernetes.io/instance", newVirtualMachineInstanceName(e)),
			k8s.WithLabel("app.kubernetes.io/part-of", "etcd"),
		),
		k8s.WithUserData(status.UserDataRef),
	); err != nil {
		return nil, fmt.Errorf("unable to create VirtualMachineInstance: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: vmi.Name,
		}, nil
	}
}

func (r *EtcdReconciler) finalizeVirtualMachineInstance(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, bool, error) {
	if e.Status.VirtualMachineRef == nil {
		return status, true, nil
	}

	logger := log.FromContext(ctx).WithValues(
		"object", e.Status.VirtualMachineRef.Name,
		"resource", "VirtualMachineInstance",
	)
	ctx = log.IntoContext(ctx, logger)

	if deleted, err := r.finalizeObject(
		ctx,
		e.Namespace,
		e.Status.VirtualMachineRef.Name,
		&kubevirtv1.VirtualMachineInstance{},
	); err != nil {
		return status, false, err
	} else if !deleted {
		return status, false, nil
	}
	status.VirtualMachineRef = nil
	logger.Info("VirtualMachine was finalized.")
	return status, true, nil
}
