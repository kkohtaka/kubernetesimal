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
	_ "embed"
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
	log.IntoContext(ctx, logger)

	var e kubernetesimalv1alpha1.Etcd
	if err := r.Get(ctx, req.NamespacedName, &e); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch Etcd")
		return ctrl.Result{}, err
	}

	if e.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&e, finalizerName) {
			controllerutil.AddFinalizer(&e, finalizerName)
			if err := r.Update(ctx, &e); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	} else {
		if controllerutil.ContainsFinalizer(&e, finalizerName) {
			if deleted, err := r.deleteExternalResources(ctx, &e); err != nil {
				return ctrl.Result{}, err
			} else if !deleted {
				return ctrl.Result{}, nil
			}

			controllerutil.RemoveFinalizer(&e, finalizerName)
			if err := r.Update(ctx, &e); err != nil {
				return ctrl.Result{}, err
			}
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
	if deleted, err := r.deleteSSHKeyPairSecret(ctx, e); err != nil {
		return false, err
	} else if !deleted {
		return false, nil
	}
	if deleted, err := r.deleteVirtualMachineInstance(ctx, e); err != nil {
		return false, err
	} else if !deleted {
		return false, nil
	}
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
			logger.Info("Secret has been deleted")
			return true, nil
		}
		logger.Error(err, "unable to get Secret")
		return false, err
	}
	if sshKeyPair.DeletionTimestamp.IsZero() {
		if err := r.Client.Delete(ctx, &sshKeyPair, &client.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("Secret has been deleted")
				return true, nil
			}
			logger.Error(err, "unable to delete Secret")
			return false, err
		}
		logger.Info("start deleting Secret")
	} else {
		logger.Info("Secret is beeing deleted")
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
			logger.Info("VirtualMachineInstance has been deleted")
			return true, nil
		}
		logger.Error(err, "unable to get VirtualMachineInstance")
		return false, err
	}
	if vmi.DeletionTimestamp.IsZero() {
		if err := r.Client.Delete(ctx, &vmi, &client.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("VirtualMachineInstance has been deleted")
				return true, nil
			}
			logger.Error(err, "unable to delete VirtualMachineInstance")
			return false, err
		}
		logger.Info("start deleting VirtualMachineInstance")
	} else {
		logger.Info("VirtualMachineInstance is beeing deleted")
	}
	return false, nil
}

func (r *EtcdReconciler) reconcileExternalResources(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	spec kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (kubernetesimalv1alpha1.EtcdStatus, error) {
	sshPrivateKeyRef, sshPublicKeyRef, err := r.reconcileSSHKeyPair(ctx, e, spec, status)
	if err != nil {
		return status, fmt.Errorf("unable to prepare an SSH keypair: %w", err)
	}
	status.SSHPrivateKeyRef = sshPrivateKeyRef
	status.SSHPublicKeyRef = sshPublicKeyRef

	userDataRef, err := r.reconcileUserData(ctx, e, spec, status)
	if err != nil {
		return status, fmt.Errorf("unable to prepare a userdata: %w", err)
	}
	status.UserDataRef = userDataRef

	vmiRef, err := r.reconcileVirtualMachineInstance(ctx, e, spec, status)
	if err != nil {
		return status, fmt.Errorf("unable to prepare a virtual machine instance: %w", err)
	}
	status.VirtualMachineRef = vmiRef

	serviceRef, err := r.reconcileService(ctx, e, spec, status)
	if err != nil {
		return status, fmt.Errorf("unable to prepare a service: %w", err)
	}
	status.ServiceRef = serviceRef

	if err := r.reconcileEtcdMember(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to prepare an etcd member: %w", err)
	}

	if err := r.probeEtcdMember(ctx, e, spec, status); err != nil {
		return status, fmt.Errorf("unable to probe an etcd member: %w", err)
	}

	return status, nil
}

const (
	sshKeyPairKeyPrivateKey = "private-key"
	sshKeyPairKeyPublicKey  = "public-key"
)

func (r *EtcdReconciler) reconcileSSHKeyPair(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.SecretKeySelector, *corev1.SecretKeySelector, error) {
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
				return nil, nil, fmt.Errorf("unable to get a Secret for an SSH keypair: %w", err)
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
		return nil, nil, fmt.Errorf("unable to create an SSH keypair: %w", err)
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
		k8s.WithDataWithKey(sshKeyPairKeyPrivateKey, privateKey),
		k8s.WithDataWithKey(sshKeyPairKeyPublicKey, publicKey),
	); err != nil {
		return nil, nil, fmt.Errorf("unable to create a Secret for an SSH keypair: %w", err)
	} else {
		return &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: sshKeyPairKeyPrivateKey,
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
	//go:embed cloud-config.tmpl
	cloudConfigTemplate string
)

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
		return nil, fmt.Errorf("unable to get an SSH public key: %w", err)
	}

	buf := bytes.Buffer{}
	tmpl, err := template.New("cloud-init").Parse(cloudConfigTemplate)
	if err != nil {
		return nil, fmt.Errorf("unable to parse a template of cloud-config: %w", err)
	}
	if err := tmpl.Execute(
		&buf,
		&struct {
			AuthorizedKeys []string
		}{
			AuthorizedKeys: []string{string(publicKey)},
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
		k8s.WithDataWithKey("userdata", buf.Bytes()),
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

func (r *EtcdReconciler) reconcileService(
	ctx context.Context,
	e *kubernetesimalv1alpha1.Etcd,
	_ kubernetesimalv1alpha1.EtcdSpec,
	status kubernetesimalv1alpha1.EtcdStatus,
) (*corev1.LocalObjectReference, error) {
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
		k8s_service.WithTargetPort("ssh", 22),
		k8s_service.WithSelector("app.kubernetes.io/name", "virtualmachineimage"),
		k8s_service.WithSelector("app.kubernetes.io/instance", newVirtualMachineInstanceName(e)),
		k8s_service.WithSelector("app.kubernetes.io/part-of", "etcd"),
	); err != nil {
		return nil, fmt.Errorf("unable to create Service: %w", err)
	} else {
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
	privateKey, err := k8s.GetValueFromSecretKeySelector(
		ctx,
		r.Client,
		e.Namespace,
		status.SSHPrivateKeyRef,
	)
	if err != nil {
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
		return err
	}
	if service.Spec.ClusterIP == "" {
		return fmt.Errorf("cluster ip of service %s/%s isn't assigned yet", e.Namespace, status.ServiceRef.Name)
	}
	var port int32
	for i := range service.Spec.Ports {
		if service.Spec.Ports[i].Name == "ssh" {
			port = service.Spec.Ports[i].TargetPort.IntVal
			break
		}
	}
	if port == 0 {
		return fmt.Errorf("port of service %s/%s isn't assigned yet", e.Namespace, status.ServiceRef.Name)
	}

	client, closer, err := ssh.StartSSHConnection(ctx, privateKey, service.Spec.ClusterIP, int(port))
	if err != nil {
		return err
	}
	defer closer()

	if err := ssh.RunCommandOverSSHSession(ctx, client, "echo"); err != nil {
		return err
	}
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

	// status.IP = ""
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
			// for i := range vmi.Status.Interfaces {
			// 	if vmi.Status.Interfaces[i].Name == "default" {
			// 		status.IP = vmi.Status.Interfaces[i].IP
			// 		break
			// 	}
			// }

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
			logger.Error(err, "unable to update status")
			return ctrl.Result{}, err
		}
		logger.Info("status is updated")
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
