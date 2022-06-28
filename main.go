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
	"github.com/kkohtaka/kubernetesimal/controllers"
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

	if err = (&controllers.EtcdReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: provider.Tracer("etcd-controller"),
		Expectations: expectations.NewUIDTrackingControllerExpectations(
			expectations.NewControllerExpectations(),
		),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Etcd")
		os.Exit(1)
	}
	if err = (&controllers.EtcdProber{
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
	if err = (&controllers.EtcdNodeReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: provider.Tracer("etcdnode-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdNode")
		os.Exit(1)
	}
	if err = (&controllers.EtcdNodeProber{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: provider.Tracer("etcdnode-prober"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create prober", "prober", "EtcdNode")
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
