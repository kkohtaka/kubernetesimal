/*
MIT License

Copyright (c) 2022 Kazumasa Kohtaka

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package etcdnode

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
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
	defaultEtcdadmReleaseURL = "https://github.com/kubernetes-sigs/etcdadm/releases/download"
)

var (
	defaultEtcdadmVersion = "0.1.5"

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
		&spec.SSHPublicKeyRef,
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
		&spec.CACertificateRef,
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
		&spec.CAPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.NewRequeueError("waiting for a CA private key prepared").Wrap(err)
		}
		return nil, fmt.Errorf("unable to get a CA private key: %w", err)
	}

	var loginPassword string
	if spec.LoginPasswordSecretKeySelector != nil {
		if v, err := k8s_secret.GetValueFromSecretKeySelector(
			ctx,
			c,
			obj.GetNamespace(),
			spec.LoginPasswordSecretKeySelector,
		); err != nil {
			return nil, fmt.Errorf("unable to get a login password: %w", err)
		} else {
			loginPassword = string(v)
		}
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

	etcdVersion := spec.Version
	if etcdVersion == "" {
		etcdVersion = defaultEtcdVersion
	}

	startClusterScriptBuf := bytes.Buffer{}
	startClusterScriptTmpl, err := template.New("start-cluster.sh.tmpl").Funcs(sprig.FuncMap()).ParseFS(
		cloudConfigTemplates,
		"templates/start-cluster.sh.tmpl",
	)
	if err != nil {
		return nil, fmt.Errorf("unable to parse a template of start-cluster.sh: %w", err)
	}
	if err := startClusterScriptTmpl.Execute(
		&startClusterScriptBuf,
		&struct {
			EtcdadmReleaseURL string
			EtcdadmVersion    string
			EtcdVersion       string
			ServiceName       string
			ExtraSANs         string
		}{
			EtcdadmReleaseURL: defaultEtcdadmReleaseURL,
			EtcdadmVersion:    defaultEtcdadmVersion,
			EtcdVersion:       etcdVersion,
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
		return nil, fmt.Errorf("unable to render start-cluster.sh from a template: %w", err)
	}

	joinClusterScriptBuf := bytes.Buffer{}
	joinClusterScriptTmpl, err := template.New("join-cluster.sh.tmpl").Funcs(sprig.FuncMap()).ParseFS(
		cloudConfigTemplates,
		"templates/join-cluster.sh.tmpl",
	)
	if err != nil {
		return nil, fmt.Errorf("unable to parse a template of join-cluster.sh: %w", err)
	}
	if err := joinClusterScriptTmpl.Execute(
		&joinClusterScriptBuf,
		&struct {
			EtcdadmReleaseURL  string
			EtcdadmVersion     string
			EtcdVersion        string
			ServiceName        string
			ExtraSANs          string
			EtcdClientEndpoint string
		}{
			EtcdadmReleaseURL: defaultEtcdadmReleaseURL,
			EtcdadmVersion:    defaultEtcdadmVersion,
			EtcdVersion:       etcdVersion,
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
			EtcdClientEndpoint: fmt.Sprintf("https://%s:%d", service.Spec.ClusterIP, servicePortEtcd),
		},
	); err != nil {
		return nil, fmt.Errorf("unable to render join-cluster.sh from a template: %w", err)
	}

	leaveClusterScriptBuf := bytes.Buffer{}
	leaveClusterScriptTmpl, err := template.New("leave-cluster.sh.tmpl").Funcs(sprig.FuncMap()).ParseFS(
		cloudConfigTemplates,
		"templates/leave-cluster.sh.tmpl",
	)
	if err != nil {
		return nil, fmt.Errorf("unable to parse a template of leave-cluster.sh: %w", err)
	}
	if err := leaveClusterScriptTmpl.Execute(
		&leaveClusterScriptBuf,
		&struct {
			EtcdadmReleaseURL string
			EtcdadmVersion    string
			EtcdVersion       string
		}{
			EtcdadmReleaseURL: defaultEtcdadmReleaseURL,
			EtcdadmVersion:    defaultEtcdadmVersion,
			EtcdVersion:       defaultEtcdVersion,
		},
	); err != nil {
		return nil, fmt.Errorf("unable to render leave-cluster.sh from a template: %w", err)
	}

	cloudInitBuf := bytes.Buffer{}
	cloudInitTmpl, err := template.New("cloud-init.tmpl").Funcs(sprig.FuncMap()).ParseFS(
		cloudConfigTemplates,
		"templates/cloud-init.tmpl",
	)
	if err != nil {
		return nil, fmt.Errorf("unable to parse a template of cloud-init: %w", err)
	}
	if err := cloudInitTmpl.Execute(
		&cloudInitBuf,
		&struct {
			LoginPassword               string
			AuthorizedKeys              []string
			StartClusterScript          string
			JoinClusterScript           string
			LeaveClusterScript          string
			CACertificate, CAPrivateKey string
		}{
			LoginPassword:      loginPassword,
			AuthorizedKeys:     []string{string(publicKey)},
			StartClusterScript: base64.StdEncoding.EncodeToString(startClusterScriptBuf.Bytes()),
			JoinClusterScript:  base64.StdEncoding.EncodeToString(joinClusterScriptBuf.Bytes()),
			LeaveClusterScript: base64.StdEncoding.EncodeToString(leaveClusterScriptBuf.Bytes()),
			CACertificate:      base64.StdEncoding.EncodeToString(caCertificate),
			CAPrivateKey:       base64.StdEncoding.EncodeToString(caPrivateKey),
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
	spec *kubernetesimalv1alpha1.EtcdNodeSpec,
	status *kubernetesimalv1alpha1.EtcdNodeStatus,
) (*corev1.LocalObjectReference, error) {
	var span trace.Span
	ctx, span = tracing.FromContext(ctx).Start(ctx, "reconcileVirtualMachineInstance")
	defer span.End()

	if _, vmi, err := k8s_vmi.CreateOnlyIfNotExist(
		ctx,
		c,
		newVirtualMachineInstanceName(obj),
		obj.GetNamespace(),
		k8s_object.WithLabel("app.kubernetes.io/name", "virtualmachineimage"),
		k8s_object.WithLabel("app.kubernetes.io/instance", newVirtualMachineInstanceName(obj)),
		k8s_object.WithLabel("app.kubernetes.io/part-of", "etcd"),
		k8s_object.WithOwner(obj, scheme),
		k8s_vmi.WithEphemeralVolumeSource(spec.ImagePersistentVolumeClaimRef.Name),
		k8s_vmi.WithUserDataSecret(status.UserDataRef),
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

	if status.VirtualMachineInstanceRef == nil {
		return status, nil
	}

	logger := log.FromContext(ctx).WithValues(
		"object", status.VirtualMachineInstanceRef.Name,
		"resource", "VirtualMachineInstance",
	)
	ctx = log.IntoContext(ctx, logger)

	if err := finalizer.FinalizeObject(
		ctx,
		client,
		obj.GetNamespace(),
		status.VirtualMachineInstanceRef.Name,
		&kubevirtv1.VirtualMachineInstance{},
	); err != nil {
		return status, err
	}
	status.VirtualMachineInstanceRef = nil
	logger.Info("VirtualMachineInstance was finalized.")
	return status, nil
}
