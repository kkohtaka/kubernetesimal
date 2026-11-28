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

package controllers

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"reflect"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kkohtaka/kubernetesimal/controllers/etcdnode"
	"github.com/kkohtaka/kubernetesimal/etcdclient"
)

// fakeKubeVirt plays the part of KubeVirt in envtest. It reports every VirtualMachineInstance as running, as soon as
// it sees it, unless the namespace of the object is held.
type fakeKubeVirt struct {
	client client.Client

	mu   sync.Mutex
	held map[string]bool
}

func newFakeKubeVirt(c client.Client) *fakeKubeVirt {
	return &fakeKubeVirt{client: c, held: map[string]bool{}}
}

// Hold keeps the virtual machines of a namespace from starting, until Release is called.
func (k *fakeKubeVirt) Hold(namespace string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.held[namespace] = true
}

// Release starts the virtual machines of a namespace that Hold kept from starting.
func (k *fakeKubeVirt) Release(ctx context.Context, namespace string) error {
	k.mu.Lock()
	delete(k.held, namespace)
	k.mu.Unlock()

	var vmis kubevirtv1.VirtualMachineInstanceList
	if err := k.client.List(ctx, &vmis, client.InNamespace(namespace)); err != nil {
		return err
	}
	for i := range vmis.Items {
		if _, err := k.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&vmis.Items[i])}); err != nil {
			return err
		}
	}
	return nil
}

func (k *fakeKubeVirt) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	k.mu.Lock()
	held := k.held[req.Namespace]
	k.mu.Unlock()
	if held {
		return ctrl.Result{}, nil
	}

	var vmi kubevirtv1.VirtualMachineInstance
	if err := k.client.Get(ctx, req.NamespacedName, &vmi); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !vmi.DeletionTimestamp.IsZero() || vmi.Status.Phase == kubevirtv1.Running {
		return ctrl.Result{}, nil
	}

	vmi.Status.Phase = kubevirtv1.Running
	if err := k.client.Update(ctx, &vmi); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (k *fakeKubeVirt) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("fake-kubevirt").
		For(&kubevirtv1.VirtualMachineInstance{}).
		Complete(k)
}

// commandCall is one command that the operator ran on a machine.
type commandCall struct {
	PrivateKey []byte
	Address    string
	Port       int
	Command    string
}

// fakeCommandRunner stands in for SSH. It records every command, and answers it with what the test decides. Until a
// test decides otherwise, every command succeeds.
type fakeCommandRunner struct {
	mu     sync.Mutex
	calls  []commandCall
	answer func(commandCall) error
}

var _ etcdnode.CommandRunner = (*fakeCommandRunner)(nil)

func newFakeCommandRunner() *fakeCommandRunner {
	return &fakeCommandRunner{}
}

func (r *fakeCommandRunner) Run(_ context.Context, privateKey []byte, address string, port int, command string) error {
	call := commandCall{
		PrivateKey: append([]byte(nil), privateKey...),
		Address:    address,
		Port:       port,
		Command:    command,
	}

	r.mu.Lock()
	r.calls = append(r.calls, call)
	answer := r.answer
	r.mu.Unlock()

	if answer != nil {
		return answer(call)
	}
	return nil
}

// Answer decides what the commands return from now on. A nil function makes every command succeed.
func (r *fakeCommandRunner) Answer(answer func(commandCall) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.answer = answer
}

// CallsTo returns the commands that were run on the machine at an address, in order.
func (r *fakeCommandRunner) CallsTo(address string) []commandCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	var calls []commandCall
	for _, call := range r.calls {
		if call.Address == address {
			calls = append(calls, call)
		}
	}
	return calls
}

// etcdCall is one request that the operator made to etcd. Target is the address or the URL that it was made to.
type etcdCall struct {
	Method    string
	Target    string
	TLSConfig *tls.Config
}

const (
	etcdMethodHealth       = "Health"
	etcdMethodMemberList   = "MemberList"
	etcdMethodMemberStatus = "MemberStatus"
)

// fakeEtcdClient stands in for etcd. It records every request, and answers it with what the test decides. Until a
// test decides otherwise, nothing answers: no address is healthy, and no cluster lists its members.
type fakeEtcdClient struct {
	mu           sync.Mutex
	calls        []etcdCall
	health       func(address string) (bool, error)
	memberList   func(address string) ([]etcdclient.Member, error)
	memberStatus func(clientURL string) error
}

var _ etcdclient.Interface = (*fakeEtcdClient)(nil)

func newFakeEtcdClient() *fakeEtcdClient {
	return &fakeEtcdClient{}
}

func (c *fakeEtcdClient) record(method, target string, tlsConfig *tls.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, etcdCall{Method: method, Target: target, TLSConfig: tlsConfig})
}

func (c *fakeEtcdClient) Health(_ context.Context, address string, tlsConfig *tls.Config) (bool, error) {
	c.record(etcdMethodHealth, address, tlsConfig)

	c.mu.Lock()
	health := c.health
	c.mu.Unlock()
	if health == nil {
		return false, nil
	}
	return health(address)
}

func (c *fakeEtcdClient) MemberList(
	_ context.Context,
	address string,
	tlsConfig *tls.Config,
) ([]etcdclient.Member, error) {
	c.record(etcdMethodMemberList, address, tlsConfig)

	c.mu.Lock()
	memberList := c.memberList
	c.mu.Unlock()
	if memberList == nil {
		return nil, errors.New("unable to list etcd members: the fake cluster does not answer")
	}
	return memberList(address)
}

func (c *fakeEtcdClient) MemberStatus(_ context.Context, clientURL string, tlsConfig *tls.Config) error {
	c.record(etcdMethodMemberStatus, clientURL, tlsConfig)

	c.mu.Lock()
	memberStatus := c.memberStatus
	c.mu.Unlock()
	if memberStatus == nil {
		return errors.New("the fake member does not answer")
	}
	return memberStatus(clientURL)
}

// AnswerHealth decides which addresses are healthy from now on.
func (c *fakeEtcdClient) AnswerHealth(health func(address string) (bool, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health = health
}

// AnswerMemberList decides what the cluster behind an address lists as its members from now on.
func (c *fakeEtcdClient) AnswerMemberList(memberList func(address string) ([]etcdclient.Member, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memberList = memberList
}

// AnswerMemberStatus decides which client URLs of members answer from now on.
func (c *fakeEtcdClient) AnswerMemberStatus(memberStatus func(clientURL string) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memberStatus = memberStatus
}

// CallsTo returns the requests of one kind that were made to an address or a URL, in order.
func (c *fakeEtcdClient) CallsTo(method, target string) []etcdCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	var calls []etcdCall
	for _, call := range c.calls {
		if call.Method == method && call.Target == target {
			calls = append(calls, call)
		}
	}
	return calls
}

// resetFakes makes the fakes answer as they do when the suite starts. It keeps what they recorded, because objects of
// earlier tests stay in the API server and their addresses do not repeat.
func resetFakes() {
	writes.Reset()
	machines.Answer(nil)
	etcdCluster.AnswerHealth(nil)
	etcdCluster.AnswerMemberList(nil)
	etcdCluster.AnswerMemberStatus(nil)
}

// fakeWriteFailures makes the operator's writes to the API server fail, for the kinds and namespaces that a test
// names. It stands for everything that can keep the operator from creating or updating an object: an API server
// that is overloaded, a quota, an admission webhook that refuses. Writes of the status are not affected.
type fakeWriteFailures struct {
	mu       sync.Mutex
	failing  map[string]bool
	attempts map[string]int
}

func newFakeWriteFailures() *fakeWriteFailures {
	return &fakeWriteFailures{failing: map[string]bool{}, attempts: map[string]int{}}
}

func writeKey(namespace, kind string) string {
	return namespace + "/" + kind
}

// Fail makes every creation and update of objects of a kind in a namespace fail, until Reset is called. The kind is
// the name of the Go type, such as "Service".
func (w *fakeWriteFailures) Fail(namespace, kind string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failing[writeKey(namespace, kind)] = true
}

// Attempts returns how many writes of a kind in a namespace have failed.
func (w *fakeWriteFailures) Attempts(namespace, kind string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.attempts[writeKey(namespace, kind)]
}

// Reset lets every write through again.
func (w *fakeWriteFailures) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failing = map[string]bool{}
}

func (w *fakeWriteFailures) check(obj client.Object) error {
	kind := reflect.TypeOf(obj).Elem().Name()
	key := writeKey(obj.GetNamespace(), kind)

	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.failing[key] {
		return nil
	}
	w.attempts[key]++
	return apierrors.NewInternalError(fmt.Errorf("the test makes writes of %s objects fail", kind))
}

// Funcs returns what the client of the operator calls instead of writing.
func (w *fakeWriteFailures) Funcs() interceptor.Funcs {
	return interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if err := w.check(obj); err != nil {
				return err
			}
			return c.Create(ctx, obj, opts...)
		},
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if err := w.check(obj); err != nil {
				return err
			}
			return c.Update(ctx, obj, opts...)
		},
		Patch: func(
			ctx context.Context,
			c client.WithWatch,
			obj client.Object,
			patch client.Patch,
			opts ...client.PatchOption,
		) error {
			if err := w.check(obj); err != nil {
				return err
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	}
}
