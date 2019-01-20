/*
Copyright 2019 Kazumasa Kohtaka <kkohtaka@gmail.com>.

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

package packetdevice

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/packethost/packngo"
	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	packetv1alpha1 "github.com/kkohtaka/kubernetesimal/pkg/apis/packet/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/pkg/util"
)

var log = logf.Log.WithName("controller")

const (
	controllerName = "packetdevice-controller"

	defaultSecretName = "packet-secret"

	secretKeyAPIKey = "apiKey"

	defaultBillingCycle = "hourly"

	EventReasonCreated        = "Created"
	EventReasonUpdated        = "Updated"
	EventReasonDeleted        = "Deleted"
	EventReasonFailedToUpdate = "FailedToUpdate"
)

// Add creates a new PacketDevice Controller and adds it to the Manager with default RBAC. The Manager will set fields
// on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	b := record.NewBroadcaster()
	b.StartLogging(func(format string, args ...interface{}) {
		log.Info(fmt.Sprintf(format, args...))
	})
	b.StartEventWatcher(func(event *v1.Event) {
		mgr.GetClient().Create(context.TODO(), event)
	})
	return &ReconcilePacketDevice{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		recorder: b.NewRecorder(
			mgr.GetScheme(),
			v1.EventSource{Component: controllerName},
		),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to PacketDevice
	err = c.Watch(&source.Kind{Type: &packetv1alpha1.PacketDevice{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePacketDevice{}

// ReconcilePacketDevice reconciles a PacketDevice object
type ReconcilePacketDevice struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a PacketDevice object and makes changes based on the state read
// and what is in the PacketDevice.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=packet.kkohtaka.org,resources=packetdevices,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcilePacketDevice) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	device := &packetv1alpha1.PacketDevice{}
	if err := r.Get(context.TODO(), request.NamespacedName, device); err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "get device: %v", request.NamespacedName)
	}

	secret := &v1.Secret{}
	secretObjKey := types.NamespacedName{
		Namespace: device.Namespace,
		Name:      defaultSecretName,
	}
	if err := r.Get(context.TODO(), secretObjKey, secret); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "get secret: %v", secretObjKey)
	}

	packet, err := newPacketClient(secret)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "create Packet client")
	}

	if util.IsDeleted(&device.ObjectMeta) {
		if device.Status.ID != "" {
			_, err = packet.Devices.Delete(device.Status.ID)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "delete device: %v on Packet", device.Status.ID)
			}
		}

		err = newUpdater(r, device).removeFinalizer().update(context.Background())
		if err != nil {
			return reconcile.Result{Requeue: true},
				errors.Wrapf(err, "remove finalizer from device: %v", request.NamespacedName)
		}

		return reconcile.Result{}, nil
	}

	if !util.HasFinalizer(&device.ObjectMeta) {
		err = newUpdater(r, device).setFinalizer().update(context.Background())
		if err != nil {
			return reconcile.Result{Requeue: true},
				errors.Wrapf(err, "set finalizer on device: %v", request.NamespacedName)
		}
	}

	var d *packngo.Device
	if device.Status.ID == "" {
		d, _, err = packet.Devices.Create(newDeviceCreateRequest(device.Spec))
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "create device: %v on Packet", request.NamespacedName)
		}
	} else {
		d, _, err = packet.Devices.Get(device.Status.ID, nil)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "get device: %v from Packet", device.Status.ID)
		}

		if shouldUpdateDevice(d, &device.Spec) {
			d, _, err = packet.Devices.Update(device.Status.ID, newDeviceUpdateRequest(d, &device.Spec))
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "update device: %v on Packet", device.Status.ID)
			}
		}
	}

	ready := isDeviceActive(d)

	if err = newUpdater(r, device).device(d).ready(ready).update(context.TODO()); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "update device: %v", request.NamespacedName)
	}

	if !device.Status.Ready {
		return reconcile.Result{
			RequeueAfter: 15 * time.Second,
		}, nil
	}

	return reconcile.Result{}, nil
}

func newPacketClient(secret *v1.Secret) (*packngo.Client, error) {
	var (
		apiKey []byte
		ok     bool
	)
	if apiKey, ok = secret.Data[secretKeyAPIKey]; !ok {
		return nil, errors.Errorf("secret %v/%v doesn't contain a key %v", secret.Namespace, secret.Name, secretKeyAPIKey)
	}

	return packngo.NewClientWithAuth("", string(apiKey), nil), nil
}

func newDeviceCreateRequest(spec packetv1alpha1.PacketDeviceSpec) *packngo.DeviceCreateRequest {
	if spec.BillingCycle == "" {
		spec.BillingCycle = defaultBillingCycle
	}
	return &packngo.DeviceCreateRequest{
		ProjectID:    spec.ProjectID,
		Facility:     []string{spec.Facility},
		Plan:         spec.Plan,
		Hostname:     spec.Hostname,
		OS:           spec.OS,
		BillingCycle: spec.BillingCycle,
	}
}

func newDeviceUpdateRequest(
	d *packngo.Device,
	spec *packetv1alpha1.PacketDeviceSpec,
) *packngo.DeviceUpdateRequest {
	req := &packngo.DeviceUpdateRequest{}
	if d.Hostname != spec.Hostname {
		req.Hostname = &spec.Hostname
	}
	return req
}

func shouldUpdateDevice(d *packngo.Device, spec *packetv1alpha1.PacketDeviceSpec) bool {
	if d.Hostname != spec.Hostname {
		return true
	}
	return false
}

func isDeviceActive(d *packngo.Device) bool {
	return d.State == string(packetv1alpha1.StateActive)
}

type updater struct {
	r        *ReconcilePacketDevice
	old, new *packetv1alpha1.PacketDevice
}

func newUpdater(r *ReconcilePacketDevice, d *packetv1alpha1.PacketDevice) *updater {
	u := updater{
		r:   r,
		old: d.DeepCopy(),
		new: d,
	}
	return &u
}

func (u *updater) device(d *packngo.Device) *updater {
	u.new.Status.State = packetv1alpha1.StringToState(d.State)
	u.new.Status.ID = d.ID

	u.new.Status.IPAddresses = make([]packetv1alpha1.IPAddress, len(d.Network))
	for i := range d.Network {
		ipAddress := d.Network[i]
		u.new.Status.IPAddresses[i] = packetv1alpha1.IPAddress{
			ID:            ipAddress.ID,
			Address:       ipAddress.Address,
			Gateway:       ipAddress.Gateway,
			Network:       ipAddress.Network,
			AddressFamily: ipAddress.AddressFamily,
			Netmask:       ipAddress.Netmask,
			Public:        ipAddress.Public,
		}
	}

	return u
}

func (u *updater) ready(ready bool) *updater {
	u.new.Status.Ready = ready
	return u
}

func (u *updater) setFinalizer() *updater {
	util.SetFinalizer(&u.new.ObjectMeta)
	return u
}

func (u *updater) removeFinalizer() *updater {
	util.RemoveFinalizer(&u.new.ObjectMeta)
	return u
}

func (u *updater) update(ctx context.Context) error {
	if reflect.DeepEqual(u.old, u.new) {
		return nil
	}
	if err := u.r.Update(ctx, u.new); err != nil {
		u.r.recorder.Eventf(
			u.new, v1.EventTypeWarning, EventReasonFailedToUpdate,
			"Failed to update PacketDevice %s/%s", u.new.Namespace, u.new.Name)
		return err
	}
	if util.IsDeleted(&u.new.ObjectMeta) && !util.HasFinalizer(&u.new.ObjectMeta) {
		u.r.recorder.Eventf(
			u.new, v1.EventTypeNormal, EventReasonDeleted,
			"Deleted PacketDevice %s/%s", u.new.Namespace, u.new.Name)
	} else {
		u.r.recorder.Eventf(
			u.new, v1.EventTypeNormal, EventReasonUpdated,
			"Updated PacketDevice %s/%s", u.new.Namespace, u.new.Name)
	}
	return nil
}
