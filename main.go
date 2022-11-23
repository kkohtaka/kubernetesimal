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

package main

import (
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	kubevirtv1 "kubevirt.io/api/core/v1"

	kubernetesimalv1alpha1 "github.com/kkohtaka/kubernetesimal/api/v1alpha1"
	"github.com/kkohtaka/kubernetesimal/controller/expectations"
	"github.com/kkohtaka/kubernetesimal/controllers/etcd"
	"github.com/kkohtaka/kubernetesimal/controllers/etcdnode"
	"github.com/kkohtaka/kubernetesimal/controllers/etcdnodedeployment"
	"github.com/kkohtaka/kubernetesimal/controllers/etcdnodeset"
	"github.com/kkohtaka/kubernetesimal/observability/tracing"
	//+kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()

	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(kubernetesimalv1alpha1.AddToScheme(scheme))

	utilruntime.Must(kubevirtv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr            string
		enableLeaderElection   bool
		probeAddr              string
		otlpAddr, otlpGRPCAddr string
		configFile             string
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&otlpAddr, "otel-collector-address", "", "The address to send traces to over HTTP.")
	flag.StringVar(&otlpGRPCAddr, "otel-collector-grpc-address", "", "The address to send traces to over gRPC.")
	flag.StringVar(&configFile, "config", "",
		"The controller will load its initial configuration from this file. "+
			"Omit this flag to use the default configuration values. "+
			"Command-line flags override configuration from this file.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	var err error
	ctrlConfig := kubernetesimalv1alpha1.KubernetesimalConfig{}
	ctrlOpts := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b197ccb6.kkohtaka.org",
	}
	if configFile != "" {
		ctrlOpts, err = ctrlOpts.AndFrom(ctrl.ConfigFile().AtPath(configFile).OfKind(&ctrlConfig))
		if err != nil {
			setupLog.Error(err, "unable to load the config file")
			os.Exit(1)
		}
	}

	ctx := ctrl.SetupSignalHandler()

	if otlpAddr == "" && ctrlConfig.Tracing.OTELCollectorAddress != "" {
		otlpAddr = ctrlConfig.Tracing.OTELCollectorAddress
	}
	if otlpAddr == "" && ctrlConfig.Tracing.OTELCollectorGRPCAddress != "" {
		otlpGRPCAddr = ctrlConfig.Tracing.OTELCollectorGRPCAddress
	}

	provider, err := tracing.NewTracerProvider(ctx, otlpAddr, otlpGRPCAddr)
	if err != nil {
		setupLog.Error(err, "unable to start trace provider")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrlOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&etcd.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: provider.Tracer("etcd-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Etcd")
		os.Exit(1)
	}
	if err = (&etcd.Prober{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: provider.Tracer("etcd-prober"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create prober", "prober", "Etcd")
		os.Exit(1)
	}
	if err = (&kubernetesimalv1alpha1.Etcd{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Etcd")
		os.Exit(1)
	}
	if err = (&etcdnode.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: provider.Tracer("etcdnode-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdNode")
		os.Exit(1)
	}
	if err = (&etcdnode.Prober{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: provider.Tracer("etcdnode-prober"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create prober", "prober", "EtcdNode")
		os.Exit(1)
	}
	if err = (&etcdnodeset.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: provider.Tracer("etcdnodeset-reconciler"),
		Expectations: expectations.NewUIDTrackingControllerExpectations(
			expectations.NewControllerExpectations(),
		),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdNodeSet")
		os.Exit(1)
	}
	if err = (&etcdnodedeployment.Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: provider.Tracer("etcdnodedeployment-reconciler"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdNodeDeployment")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
