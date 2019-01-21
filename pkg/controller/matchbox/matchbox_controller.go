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

package matchbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	packetv1alpha1 "github.com/kkohtaka/kubernetesimal/pkg/apis/packet/v1alpha1"
	servicesv1alpha1 "github.com/kkohtaka/kubernetesimal/pkg/apis/services/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/pkg/util"
)

var log = logf.Log.WithName("controller")

const (
	controllerName = "matchbox-controller"

	eventReasonCreated        = "Created"
	eventReasonUpdated        = "Updated"
	eventReasonDeleted        = "Deleted"
	eventReasonFailedToUpdate = "FailedToUpdate"
)

// Add creates a new Matchbox Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
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
	return &ReconcileMatchbox{
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

	// Watch for changes to Matchbox
	err = c.Watch(&source.Kind{Type: &servicesv1alpha1.Matchbox{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes of PacketDevice owned by the Matchbox and trigger a Reconcile for the owner
	err = c.Watch(
		&source.Kind{Type: &packetv1alpha1.PacketDevice{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &servicesv1alpha1.Matchbox{},
		})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileMatchbox{}

// ReconcileMatchbox reconciles a Matchbox object
type ReconcileMatchbox struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a Matchbox object and makes changes based on the state read
// and what is in the Matchbox.Spec
// +kubebuilder:rbac:groups=packet.kkohtaka.org,resources=packetdevices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=services.kkohtaka.org,resources=matchboxes,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileMatchbox) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Matchbox instance
	m := &servicesv1alpha1.Matchbox{}
	err := r.Get(context.TODO(), request.NamespacedName, m)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if util.IsDeleted(&m.ObjectMeta) {
		err = newUpdater(r, m).removeFinalizer().update(context.Background())
		if err != nil {
			return reconcile.Result{Requeue: true},
				errors.Wrapf(err, "remove finalizer from Matchbox: %v", request.NamespacedName)
		}

		return reconcile.Result{}, nil
	}

	if !util.HasFinalizer(&m.ObjectMeta) {
		err = newUpdater(r, m).setFinalizer().update(context.Background())
		if err != nil {
			return reconcile.Result{Requeue: true},
				errors.Wrapf(err, "set finalizer from Matchbox: %v", request.NamespacedName)
		}
	}

	if !isSetPacketDeviceName(m) {
		pdName, err := generatePacketDeviceName(m)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "generate PacketDevice name for Matchbox: %s", request.NamespacedName)
		}

		err = newUpdater(r, m).packetDeviceName(pdName).update(context.TODO())
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "update Matchbox: %s", request.NamespacedName)
		}
		return reconcile.Result{Requeue: true}, nil
	}

	objKey := types.NamespacedName{
		Namespace: m.Namespace,
		Name:      getPacketDeviceName(m),
	}
	var pd *packetv1alpha1.PacketDevice
	err = r.Get(context.TODO(), objKey, pd)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return reconcile.Result{}, errors.Wrapf(err, "get PacketDevice: %s", objKey)
		}
	}

	if pd == nil {
		pd, err = newPacketDevice(m)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "generate PacketDevice for Matchbox: %s", request.NamespacedName)
		}
		if err := controllerutil.SetControllerReference(m, pd, r.scheme); err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "set owner reference: %s on PacketDevice", request.NamespacedName)
		}
		if err := r.Create(context.TODO(), pd); err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "create PacketDevice: %s/%s", pd.Namespace, pd.Name)
		}
	}

	ready := pd.Status.Ready

	if err := newUpdater(r, m).ready(ready).update(context.TODO()); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "update Matchbox: %s", request.NamespacedName)
	}

	return reconcile.Result{}, nil
}

func isSetPacketDeviceName(m *servicesv1alpha1.Matchbox) bool {
	return m.Status.PacketDeviceRef.Name == ""
}

func getPacketDeviceName(m *servicesv1alpha1.Matchbox) string {
	return m.Status.PacketDeviceRef.Name
}

func setPacketDeviceName(m *servicesv1alpha1.Matchbox, name string) {
	m.Status.PacketDeviceRef.Name = name
}

func generatePacketDeviceName(m *servicesv1alpha1.Matchbox) (string, error) {
	h, err := newRandomHex(8)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", m.Name, h), nil
}

func newRandomHex(n int) (string, error) {
	b := make([]byte, n)
	offset := 0
	for offset < n {
		var (
			nread int
			err   error
		)
		if nread, err = rand.Read(b[offset:]); err != nil {
			return "", err
		}
		offset = offset + nread
	}
	return hex.EncodeToString(b), nil
}

func newMatchboxHostname(m *servicesv1alpha1.Matchbox) (string, error) {
	h, err := newRandomHex(8)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%s", m.Namespace, m.Name, h), nil
}

func newMatchboxUserData() (string, error) {
	return "", nil
}

func newPacketDevice(m *servicesv1alpha1.Matchbox) (*packetv1alpha1.PacketDevice, error) {
	var err error
	pd := packetv1alpha1.PacketDevice{}
	pd.Spec.Hostname, err = newMatchboxHostname(m)
	if err != nil {
		return nil, errors.Wrap(err, "generate a hostname")
	}
	ignitionConfig := newIgnitionConfig()
	userData, err := json.Marshal(&ignitionConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal an ignition config: %+v", ignitionConfig)
	}
	pd.Spec.UserData = string(userData)
	// TODO: Make the following properties configureable
	pd.Spec.Facility = "nrt1"
	pd.Spec.Plan = "t1.small.x86"
	pd.Spec.OS = "coreos_stable"
	pd.Spec.BillingCycle = "hourly"
	return &pd, nil
}

type updater struct {
	r        *ReconcileMatchbox
	old, new *servicesv1alpha1.Matchbox
}

func newUpdater(r *ReconcileMatchbox, m *servicesv1alpha1.Matchbox) *updater {
	u := updater{
		r:   r,
		old: m.DeepCopy(),
		new: m,
	}
	return &u
}

func (u *updater) setFinalizer() *updater {
	util.SetFinalizer(&u.new.ObjectMeta)
	return u
}

func (u *updater) removeFinalizer() *updater {
	util.RemoveFinalizer(&u.new.ObjectMeta)
	return u
}

func (u *updater) ready(ready bool) *updater {
	u.new.Status.Ready = ready
	return u
}

func (u *updater) packetDeviceName(pdName string) *updater {
	u.new.Status.PacketDeviceRef.Name = pdName
	return u
}

func (u *updater) update(ctx context.Context) error {
	if reflect.DeepEqual(u.old, u.new) {
		return nil
	}
	if err := u.r.Update(ctx, u.new); err != nil {
		u.r.recorder.Eventf(
			u.new, v1.EventTypeWarning, eventReasonFailedToUpdate,
			"Failed to update PacketDevice %s/%s", u.new.Namespace, u.new.Name)
		return err
	}
	if util.IsDeleted(&u.new.ObjectMeta) && !util.HasFinalizer(&u.new.ObjectMeta) {
		u.r.recorder.Eventf(
			u.new, v1.EventTypeNormal, eventReasonDeleted,
			"Deleted PacketDevice %s/%s", u.new.Namespace, u.new.Name)
	} else {
		u.r.recorder.Eventf(
			u.new, v1.EventTypeNormal, eventReasonUpdated,
			"Updated PacketDevice %s/%s", u.new.Namespace, u.new.Name)
	}
	return nil
}
