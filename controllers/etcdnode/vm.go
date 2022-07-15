package etcdnode

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"strings"
	"text/template"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/errors"
	"github.com/kkohtaka/kubernetesimal/controller/finalizer"
	k8s_object "github.com/kkohtaka/kubernetesimal/k8s/object"
	k8s_secret "github.com/kkohtaka/kubernetesimal/k8s/secret"
	k8s_vmi "github.com/kkohtaka/kubernetesimal/k8s/vmi"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
)

var (
	//go:embed templates/*.tmpl
	cloudConfigTemplates embed.FS
)

const (
	defaultEtcdadmReleaseURL = "https://github.com/kkohtaka/etcdadm/releases/download"
)

var (
	defaultEtcdadmVersion = "2022.05.31"

	defaultEtcdVersion = "3.5.1"
)

func newUserDataName(obj client.Object) string {
	return "userdata-" + obj.GetName()
}

func newVirtualMachineInstanceName(obj client.Object) string {
	return obj.GetName()
}

func reconcileUserData(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	obj client.Object,
	spec *kubernetesimalv1alpha1.EtcdNodeSpec,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) (*corev1.LocalObjectReference, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileUserData")
	defer span.End()

	publicKey, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
		spec.SSHPublicKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewRequeueError("waiting for an SSH public key prepared").Wrap(err)
		}
		return nil, fmt.Errorf("unable to get an SSH public key: %w", err)
	}

	caCertificate, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
		spec.CACertificateRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewRequeueError("waiting for a CA certificate prepared").Wrap(err)
		}
		return nil, fmt.Errorf("unable to get a CA certificate: %w", err)
	}

	caPrivateKey, err := k8s_secret.GetValueFromSecretKeySelector(
		ctx,
		c,
		obj.GetNamespace(),
		spec.CAPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewRequeueError("waiting for a CA private key prepared").Wrap(err)
		}
		return nil, fmt.Errorf("unable to get a CA private key: %w", err)
	}

	var service corev1.Service
	if err := c.Get(
		ctx,
		types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      spec.ServiceRef.Name,
		},
		&service,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewRequeueError("waiting for the etcd Service prepared").Wrap(err)
		}
		return nil, fmt.Errorf("unable to get a service %s/%s: %w", obj.GetNamespace(), spec.ServiceRef.Name, err)
	}
	if service.Spec.ClusterIP == "" {
		return nil, errors.NewRequeueError("waiting for a cluster IP of the etcd Service prepared")
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
			return nil, errors.NewRequeueError("waiting for the etcd peer Service prepared").Wrap(err)
		}
		return nil, fmt.Errorf(
			"unable to get a peer service %s/%s: %w",
			obj.GetNamespace(),
			status.PeerServiceRef.Name,
			err,
		)
	}
	if peerService.Spec.ClusterIP == "" {
		return nil, errors.NewRequeueError("waiting for a cluster IP of the etcd peer Service prepared")
	}

	startEtcdScriptBuf := bytes.Buffer{}
	startEtcdScriptTmpl, err := template.New("start-etcd.sh.tmpl").ParseFS(
		cloudConfigTemplates,
		"templates/start-etcd.sh.tmpl",
	)
	if err != nil {
		return nil, fmt.Errorf("unable to parse a template of start-etcd.sh: %w", err)
	}
	if err := startEtcdScriptTmpl.Execute(
		&startEtcdScriptBuf,
		&struct {
			EtcdadmReleaseURL string
			EtcdadmVersion    string
			EtcdVersion       string
			ServiceName       string
			ExtraSANs         string
		}{
			EtcdadmReleaseURL: defaultEtcdadmReleaseURL,
			EtcdadmVersion:    defaultEtcdadmVersion,
			EtcdVersion:       defaultEtcdVersion,
			ServiceName:       peerService.Name,
			ExtraSANs: strings.Join(
				[]string{
					peerService.Spec.ClusterIP,
					fmt.Sprintf("%s.%s.svc", peerService.Name, peerService.Namespace),
					fmt.Sprintf("%s.%s", peerService.Name, peerService.Namespace),
					service.Spec.ClusterIP,
					fmt.Sprintf("%s.%s.svc", service.Name, service.Namespace),
					fmt.Sprintf("%s.%s", service.Name, service.Namespace),
				},
				",",
			),
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
			AuthorizedKeys              []string
			StartEtcdScript             string
			CACertificate, CAPrivateKey string
		}{
			AuthorizedKeys:  []string{string(publicKey)},
			StartEtcdScript: base64.StdEncoding.EncodeToString(startEtcdScriptBuf.Bytes()),
			CACertificate:   base64.StdEncoding.EncodeToString(caCertificate),
			CAPrivateKey:    base64.StdEncoding.EncodeToString(caPrivateKey),
		},
	); err != nil {
		return nil, fmt.Errorf("unable to render a cloud-config from a template: %w", err)
	}

	if secret, err := k8s_secret.CreateOnlyIfNotExist(
		ctx,
		obj,
		c,
		newUserDataName(obj),
		obj.GetNamespace(),
		k8s_object.WithOwner(obj, scheme),
		k8s_secret.WithDataWithKey("userdata", cloudInitBuf.Bytes()),
	); err != nil {
		return nil, fmt.Errorf("unable to create Secret: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: secret.Name,
		}, nil
	}
}

func reconcileVirtualMachineInstance(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	obj client.Object,
	_ *kubernetesimalv1alpha1.EtcdNodeSpec,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) (*corev1.LocalObjectReference, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileVirtualMachineInstance")
	defer span.End()

	if vmi, err := k8s_vmi.CreateIfNotExist(
		ctx,
		obj,
		c,
		k8s_object.WithName(newVirtualMachineInstanceName(obj)),
		k8s_object.WithNamespace(obj.GetNamespace()),
		k8s_object.WithLabel("app.kubernetes.io/name", "virtualmachineimage"),
		k8s_object.WithLabel("app.kubernetes.io/instance", newVirtualMachineInstanceName(obj)),
		k8s_object.WithLabel("app.kubernetes.io/part-of", "etcd"),
		k8s_object.WithOwner(obj, scheme),
		k8s_vmi.WithUserData(status.UserDataRef),
		k8s_vmi.WithReadinessTCPProbe(&corev1.TCPSocketAction{
			Port: intstr.FromInt(serviceContainerPortSSH),
		}),
	); err != nil {
		return nil, fmt.Errorf("unable to create VirtualMachineInstance: %w", err)
	} else {
		return &corev1.LocalObjectReference{
			Name: vmi.Name,
		}, nil
	}
}

func finalizeVirtualMachineInstance(
	ctx context.Context,
	client client.Client,
	obj client.Object,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) (*kubernetesimalv1alpha1.EtcdNodeStatus, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "finalizeVirtualMachineInstance")
	defer span.End()

	if status.VirtualMachineRef == nil {
		return status, nil
	}

	logger := log.FromContext(ctx).WithValues(
		"object", status.VirtualMachineRef.Name,
		"resource", "VirtualMachineInstance",
	)
	ctx = log.IntoContext(ctx, logger)

	if err := finalizer.FinalizeObject(
		ctx,
		client,
		obj.GetNamespace(),
		status.VirtualMachineRef.Name,
		&kubevirtv1.VirtualMachineInstance{},
	); err != nil {
		return status, err
	}
	status.VirtualMachineRef = nil
	logger.Info("VirtualMachine was finalized.")
	return status, nil
}