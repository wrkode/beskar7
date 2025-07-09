/*
Copyright 2024 The Beskar7 Authors.

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
	"context"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
	"github.com/wrkode/beskar7/api/v1beta1/webhooks"
	"github.com/wrkode/beskar7/controllers"
	internalmetrics "github.com/wrkode/beskar7/internal/metrics"
	internalredfish "github.com/wrkode/beskar7/internal/redfish"
	"github.com/wrkode/beskar7/internal/security"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(infrastructurev1beta1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

//+kubebuilder:rbac:groups=*,resources=*,verbs=*
//+kubebuilder:rbac:namespace=beskar7-system,groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=create;delete;get;list;patch;update;watch

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var enableSecurityMonitoring bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&enableSecurityMonitoring, "enable-security-monitoring", true,
		"Enable security monitoring for TLS, RBAC, and credential validation.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Initialize metrics
	internalmetrics.Init()
	setupLog.Info("Metrics initialized successfully")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: metricsAddr},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7be7e04e.cluster.x-k8s.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the future, controller-runtime will detect on its own if it is safe to enable
		// this option, meaning this flag has a chance to be removed without notice.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup controllers here
	if err = (&controllers.Beskar7MachineReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		// Use the default NewClient, which will be shared.
		RedfishClientFactory: internalredfish.NewClient,
	}).SetupWithManager(context.Background(), mgr, controller.Options{MaxConcurrentReconciles: 10}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Beskar7Machine")
		os.Exit(1)
	}

	if err = (&controllers.Beskar7ClusterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(context.Background(), mgr, controller.Options{}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Beskar7Cluster")
		os.Exit(1)
	}

	if err = (&controllers.PhysicalHostReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		RedfishClientFactory: internalredfish.NewClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PhysicalHost")
		os.Exit(1)
	}

	// Setup webhooks
	if err = (&webhooks.Beskar7ClusterWebhook{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Beskar7Cluster")
		os.Exit(1)
	}

	if err = (&webhooks.Beskar7MachineWebhook{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Beskar7Machine")
		os.Exit(1)
	}

	if err = (&webhooks.PhysicalHostWebhook{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "PhysicalHost")
		os.Exit(1)
	}

	// Setup security monitoring
	if enableSecurityMonitoring {
		kubernetesClient, err := kubernetes.NewForConfig(mgr.GetConfig())
		if err != nil {
			setupLog.Error(err, "unable to create Kubernetes client for security monitoring")
			os.Exit(1)
		}

		securityMonitor := security.NewSecurityMonitor(
			mgr.GetClient(),
			kubernetesClient,
			"beskar7-system", // Default namespace
		)

		// Start security monitoring in the background
		if err := mgr.Add(&securityMonitorManager{
			monitor: securityMonitor,
		}); err != nil {
			setupLog.Error(err, "unable to add security monitor to manager")
			os.Exit(1)
		}

		setupLog.Info("Security monitoring enabled")
	} else {
		setupLog.Info("Security monitoring disabled")
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
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// securityMonitorManager wraps the security monitor to implement manager.Runnable
type securityMonitorManager struct {
	monitor *security.SecurityMonitor
}

func (s *securityMonitorManager) Start(ctx context.Context) error {
	return s.monitor.StartMonitoring(ctx)
}
