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
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/expectations"
	"github.com/kkohtaka/kubernetesimal/controllers/etcd"
	"github.com/kkohtaka/kubernetesimal/controllers/etcdnode"
	"github.com/kkohtaka/kubernetesimal/controllers/etcdnodedeployment"
	"github.com/kkohtaka/kubernetesimal/controllers/etcdnodeset"
	//+kubebuilder:scaffold:imports
)

// This suite runs every controller of the operator against the API server of envtest, wired as main.go wires them.
//
// envtest has no KubeVirt, no virtual machines, and no etcd. Three fakes stand in for them:
//
//   - kubeVirt plays the part of KubeVirt: it reports the VirtualMachineInstance objects that the operator creates
//     as running.
//   - machines is the command runner that the operator is given instead of SSH. It records the commands.
//   - etcdCluster is the etcd client that the operator is given. Tests decide what it answers.
//
// What the fakes cannot show, such as a virtual machine that boots and an etcd that forms a cluster, is left to the
// end-to-end tests.

var (
	k8sClient client.Client
	testEnv   *envtest.Environment

	suiteCtx    context.Context
	suiteCancel context.CancelFunc

	kubeVirt    *fakeKubeVirt
	machines    *fakeCommandRunner
	etcdCluster *fakeEtcdClient
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	suiteCtx, suiteCancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("testdata", "config", "crd", "bases"),
			filepath.Join("testdata", "kubevirt"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(kubernetesimalv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(kubevirtv1.AddToScheme(scheme)).To(Succeed())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
	})
	Expect(err).NotTo(HaveOccurred())

	machines = newFakeCommandRunner()
	etcdCluster = newFakeEtcdClient()
	kubeVirt = newFakeKubeVirt(mgr.GetClient())
	Expect(kubeVirt.SetupWithManager(mgr)).To(Succeed())

	tracer := trace.NewNoopTracerProvider().Tracer("test")

	Expect((&etcd.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: tracer,
	}).SetupWithManager(mgr)).To(Succeed())
	Expect((&etcd.Prober{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Tracer:     tracer,
		EtcdClient: etcdCluster,
	}).SetupWithManager(mgr)).To(Succeed())
	Expect((&etcdnode.Reconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Tracer:        tracer,
		CommandRunner: machines,
	}).SetupWithManager(mgr)).To(Succeed())
	Expect((&etcdnode.Prober{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Tracer:     tracer,
		EtcdClient: etcdCluster,
	}).SetupWithManager(mgr)).To(Succeed())
	Expect((&etcdnodeset.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: tracer,
		Expectations: expectations.NewUIDTrackingControllerExpectations(
			expectations.NewControllerExpectations(),
		),
	}).SetupWithManager(mgr)).To(Succeed())
	Expect((&etcdnodedeployment.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: tracer,
	}).SetupWithManager(mgr)).To(Succeed())

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(suiteCtx)).To(Succeed())
	}()
}, 60)

var _ = AfterSuite(func() {
	By("stopping the controllers")
	suiteCancel()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
