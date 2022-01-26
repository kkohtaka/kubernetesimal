/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/k8s"
	k8s_service "github.com/kkohtaka/kubernetesimal/k8s/service"
	"github.com/kkohtaka/kubernetesimal/pki"
	"github.com/kkohtaka/kubernetesimal/ssh"
)

// EtcdReconciler reconciles a Etcd object
type EtcdReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernetesimal.kkohtaka.org,resources=etcds/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

const (
	finalizerName = "kubernetesimal.kkohtaka.org/finalizer"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *EtcdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("etcd", req.NamespacedName)
	ctx = log.IntoContext(ctx, logger)

	var e kubernetesimalv1alpha1.Etcd
	if err := r.Get(ctx, req.NamespacedName, &e); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if e.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&e, finalizerName) {
			controllerutil.AddFinalizer(&e, finalizerName)
			if err := r.Update(ctx, &e); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("A finalizer was set.")
			return ctrl.Result{}, nil
		}
	} else {
		if controllerutil.ContainsFinalizer(&e, finalizerName) {
			if deleted, err := r.deleteExternalResources(ctx, &e); err != nil {
				return ctrl.Result{}, err
			} else if !deleted {
				return ctrl.Result{}, nil
			}
			logger.Info("External resources were deleted.")

			controllerutil.RemoveFinalizer(&e, finalizerName)
			if err := r.Update(ctx, &e); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("The finalizer was unset.")
		}
		return ctrl.Result{}, nil
	}

	status, err := r.reconcileExternalResources(ctx, &e, e.Spec, e.Status)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.updateStatus(ctx, &e, status)
}

func (r *EtcdReconciler) deleteExternalResources(ctx context.Context, e *kubernetesimalv1alpha1.Etcd) (bool, error) {
	logger := log.FromContext(ctx)

	if deleted, err := r.deleteSSHKeyPairSecret(ctx, e); err != nil {
		return false, err
	} else if !deleted {
		return false, nil
	}
	logger.Info("SSH key-pair was finalized.")

	if deleted, err := r.deleteVirtualMachineInstance(ctx, e); err != nil {
		return false, err
	} else if !deleted {
		return false, nil
	}
	logger.Info("VirtualMachine was finalized.")

	return true, nil
}

func (r *EtcdReconciler) deleteSSHKeyPairSecret(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
) (bool, error) {
	logger := log.FromContext(ctx)

	if e.Status.SSHPrivateKeyRef == nil {
		return true, nil
	}
	key := types.NamespacedName{
		Namespace: e.Namespace,
		Name:      e.Status.SSHPrivateKeyRef.Name,
	}
	var sshKeyPair corev1.Secret
	if err := r.Client.Get(ctx, key, &sshKeyPair); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("The Secret of SSH key-pair has already been deleted.")
			return true, nil
		}
		return false, err
	}
	if sshKeyPair.DeletionTimestamp.IsZero() {
		if err := r.Client.Delete(ctx, &sshKeyPair, &client.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("The Secret of SSH key-pair has already been deleted.")
				return true, nil
			}
			return false, err
		}
		logger.Info("The Secret of SSH key-pair has started to be deleted.")
	} else {
		logger.Info("The Secret of SSH key-pair is beeing deleted.")
	}
	return false, nil
}

func (r *EtcdReconciler) deleteVirtualMachineInstance(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
) (bool, error) {
	logger := log.FromContext(ctx)

	if e.Status.VirtualMachineRef == nil {
		return true, nil
	}
	key := types.NamespacedName{
		Namespace: e.Namespace,
		Name:      e.Status.VirtualMachineRef.Name,
	}
	var vmi kubevirtv1.VirtualMachineInstance
	if err := r.Client.Get(ctx, key, &vmi); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("The VirtualMachineInstance for an etcd member has already been deleted.")
			return true, nil
		}
		return false, err
	}
	if vmi.DeletionTimestamp.IsZero() {
		if err := r.Client.Delete(ctx, &vmi, &client.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("The VirtualMachineInstance for an etcd member has already been deleted.")
				return true, nil
			}
			return false, err
		}
		logger.Info("The VirtualMachineInstance for an etcd member has started to be deleted.")
	} else {
		logger.Info("The VirtualMachineInstance for an etcd member is beeing deleted.")
	}
	return false, nil
}

func (r *EtcdReconciler) reconcileExternalResources(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	spec kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	caCertificateRef, caPrivateKeyRef, err := r.reconcileCACertificate(ctx, e, spec, status)
	if err != nil {
		return status, fmt.Errorf("unable to prepare a CA certificate: %w", err)
	}
	status.CAPrivateKeyRef = caPrivateKeyRef
	status.CACertificateRef = caCertificateRef

	if sshPrivateKeyRef, sshPublicKeyRef, err := r.reconcileSSHKeyPair(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare an SSH key-pair: %w", err)
	} else if sshPrivateKeyRef == nil || sshPublicKeyRef == nil {
		return status, nil
	} else {
		status.SSHPrivateKeyRef = sshPrivateKeyRef
		status.SSHPublicKeyRef = sshPublicKeyRef
	}

	if serviceRef, err := r.reconcileService(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a service: %w", err)
	} else if serviceRef == nil {
		return status, nil
	} else {
		status.ServiceRef = serviceRef
	}

	if userDataRef, err := r.reconcileUserData(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a userdata: %w", err)
	} else if userDataRef == nil {
		return status, nil
	} else {
		status.UserDataRef = userDataRef
	}

	if vmiRef, err := r.reconcileVirtualMachineInstance(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare a virtual machine instance: %w", err)
	} else if vmiRef == nil {
		return status, nil
	} else {
		status.VirtualMachineRef = vmiRef
	}

	switch status.Phase {
	case kubernetesimalv1alpha1.EtcdPhaseRunning:
	default:
		if err := r.reconcileEtcdMember(ctx, e, spec, status); err != nil {
			return status, fmt.Errorf("unable to prepare an etcd member: %w", err)
		}
	}

	if err := r.probeEtcdMember(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to probe an etcd member: %w", err)
	}

	return status, nil
}

func (r *EtcdReconciler) reconcileCACertificate(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.SecretKeySelector, *corev1.SecretKeySelector, error) {
	logger := log.FromContext(ctx)

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
		if err := r.Client.Get(
			ctx,
			types.NamespacedName{Namespace: e.Namespace, Name: status.CAPrivateKeyRef.Name},
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
	if secret, err := k8s.ReconcileSecret(
		ctx,
		e,
		r.Scheme,
		r.Client,
		k8s.NewObjectMeta(
			k8s.WithName(newCACertificateName(e)),
			k8s.WithNamespace(e.Namespace),
		),
		k8s.WithType(corev1.SecretTypeTLS),
		k8s.WithDataWithKey(corev1.TLSCertKey, certificate),
		k8s.WithDataWithKey(corev1.TLSPrivateKeyKey, privateKey),
	); err != nil {
		return nil, nil, fmt.Errorf("unable to prepare a Secret for a CA certificate for etcd: %w", err)
	} else {
		logger.Info("A Secret for CA certificate was prepared.")
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

const (
	sshKeyPairKeyPublicKey = "ssh-publickey"
)

func (r *EtcdReconciler) reconcileSSHKeyPair(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.SecretKeySelector, *corev1.SecretKeySelector, error) {
	logger := log.FromContext(ctx)

	if status.SSHPrivateKeyRef != nil {
		if name := status.SSHPrivateKeyRef.LocalObjectReference.Name; name != newSSHKeyPairName(e) {
			return nil, nil, fmt.Errorf("invalid Secret name %s to store an SSH private key", name)
		}
	}
	if status.SSHPublicKeyRef != nil {
		if name := status.SSHPublicKeyRef.LocalObjectReference.Name; name != newSSHKeyPairName(e) {
			return nil, nil, fmt.Errorf("invalid Secret name %s to store an SSH public key", name)
		}
	}

	var sshKeyPair corev1.Secret
	if status.SSHPrivateKeyRef != nil && status.SSHPublicKeyRef != nil {
		if err := r.Client.Get(
			ctx,
			types.NamespacedName{Namespace: e.Namespace, Name: status.SSHPrivateKeyRef.Name},
			&sshKeyPair,
		); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, nil, fmt.Errorf("unable to get a Secret for an SSH key-pair: %w", err)
			}
		} else {
			_, hasPrivateKey := sshKeyPair.Data[status.SSHPrivateKeyRef.Key]
			_, hasPublicKey := sshKeyPair.Data[status.SSHPublicKeyRef.Key]
			if hasPrivateKey && hasPublicKey {
				return status.SSHPrivateKeyRef, status.SSHPublicKeyRef, nil
			}
		}
	}

	privateKey, publicKey, err := ssh.GenerateKeyPair()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create an SSH key-pair: %w", err)
	}
	if secret, err := k8s.ReconcileSecret(
		ctx,
		e,
		r.Scheme,
		r.Client,
		k8s.NewObjectMeta(
			k8s.WithName(newSSHKeyPairName(e)),
			k8s.WithNamespace(e.Namespace),
		),
		k8s.WithType(corev1.SecretTypeSSHAuth),
		k8s.WithDataWithKey(corev1.SSHAuthPrivateKey, privateKey),
		k8s.WithDataWithKey(sshKeyPairKeyPublicKey, publicKey),
	); err != nil {
		return nil, nil, fmt.Errorf("unable to prepare a Secret for an SSH key-pair: %w", err)
	} else {
		logger.Info("A Secret for an SSH key-pair was prepared.")
		return &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: corev1.SSHAuthPrivateKey,
			},
			&corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: sshKeyPairKeyPublicKey,
			},
			nil
	}
}

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

func (r *EtcdReconciler) reconcileUserData(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.LocalObjectReference, error) {
	logger := log.FromContext(ctx)

	publicKey, err := k8s.GetValueFromSecretKeySelector(
		ctx,
		r.Client,
		e.Namespace,
		status.SSHPublicKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip reconciling userdata since SSH public key isn't prepared yet.")
			return nil, nil
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
			logger.Info("Skip reconciling userdata since CA certificate isn't prepared yet.")
			return nil, nil
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
		return nil, fmt.Errorf("unable to get an SSH public key: %w", err)
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
			logger.Info("Skip reconciling userdata since the etcd Service isn't prepared yet.")
			return nil, nil
		}
		return nil, fmt.Errorf("unable to get a service %s/%s: %w", e.Namespace, status.ServiceRef.Name, err)
	}
	if service.Spec.ClusterIP == "" {
		return nil, fmt.Errorf("cluster ip of service %s/%s isn't assigned yet.", e.Namespace, status.ServiceRef.Name)
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
		logger.Info("A Secret for a userdata was prepared.")
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
	logger := log.FromContext(ctx)

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
		logger.Info("A VirtualMachineInstance for an etcd member was prepared.")
		return &corev1.LocalObjectReference{
			Name: vmi.Name,
		}, nil
	}
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

func (r *EtcdReconciler) reconcileEtcdMember(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) error {
	logger := log.FromContext(ctx)

	privateKey, err := k8s.GetValueFromSecretKeySelector(
		ctx,
		r.Client,
		e.Namespace,
		status.SSHPrivateKeyRef,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Skip reconciling an etcd member since SSH private key isn't prepared yet.")
			return nil
		}
		return nil
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
			logger.Info("Skip reconciling an etcd member since the etcd Service isn't prepared yet.")
			return nil
		}
		return err
	}
	if service.Spec.ClusterIP == "" {
		logger.Info("Skip reconciling an etcd member since cluster ip isn't assigned yet.")
		return nil
	}
	var port int32
	for i := range service.Spec.Ports {
		if service.Spec.Ports[i].Name == "ssh" {
			port = service.Spec.Ports[i].TargetPort.IntVal
			break
		}
	}
	if port == 0 {
		logger.Info("Skip reconciling an etcd member since port of service %s/%s isn't assigned yet.")
		return nil
	}

	client, closer, err := ssh.StartSSHConnection(ctx, privateKey, service.Spec.ClusterIP, int(port))
	if err != nil {
		logger.Info(
			"Skip reconciling an etcd member since SSH port of an etcd member isn't available yet.",
			"reason", err,
		)
		return nil
	}
	defer closer()

	if err := ssh.RunCommandOverSSHSession(ctx, client, "sudo /opt/bin/start-etcd.sh"); err != nil {
		return err
	}
	logger.Info("Succeeded in executing a start-up script for an etcd member on the VirtualMachineInstance.")

	return nil
}

func (r *EtcdReconciler) probeEtcdMember(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) error {
	return nil
}

func newCACertificateName(e *kubernetesimalv1alpha1.Etcd) string {
	return "ca-" + e.Name
}

func newCACertificateIssuerName(e *kubernetesimalv1alpha1.Etcd) string {
	return e.Name
}

func newSSHKeyPairName(e *kubernetesimalv1alpha1.Etcd) string {
	return "ssh-keypair-" + e.Name
}

func newUserDataName(e *kubernetesimalv1alpha1.Etcd) string {
	return "userdata-" + e.Name
}

func newVirtualMachineInstanceName(e *kubernetesimalv1alpha1.Etcd) string {
	return e.Name
}

func newServiceName(e *kubernetesimalv1alpha1.Etcd) string {
	return e.Name
}

func (r *EtcdReconciler) updateStatus(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	status kubernetesimalv1alpha1.EtcdStatus,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	status.Phase = kubernetesimalv1alpha1.EtcdPhasePending
	if status.VirtualMachineRef != nil {
		key := types.NamespacedName{
			Name:      status.VirtualMachineRef.Name,
			Namespace: e.Namespace,
		}
		var vmi kubevirtv1.VirtualMachineInstance
		if err := r.Get(ctx, key, &vmi); err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("unable to get VirtualMachineInstance %s: %w", key, err)
			}
		} else {
			switch vmi.Status.Phase {
			case kubevirtv1.Running:
				status.Phase = kubernetesimalv1alpha1.EtcdPhaseRunning
			}
		}
	}

	if !apiequality.Semantic.DeepEqual(status, e.Status) {
		patch := client.MergeFrom(e.DeepCopy())
		e.Status = status
		if err := r.Client.Status().Patch(ctx, e, patch); err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
		logger.Info("Status was updated.")
			return ctrl.Result{}, err
		}
		logger.Info("Status was updated.")
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EtcdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubernetesimalv1alpha1.Etcd{}).
		Owns(&corev1.Secret{}).
		Owns(&kubevirtv1.VirtualMachineInstance{}).
		Complete(r)
}
