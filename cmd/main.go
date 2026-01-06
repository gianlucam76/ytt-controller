/*
Copyright 2023.

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
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	//+kubebuilder:scaffold:imports

	"github.com/gianlucam76/ytt-controller/controllers"

	"github.com/projectsveltos/libsveltos/lib/crd"
	"github.com/projectsveltos/libsveltos/lib/logsettings"
	libsveltosset "github.com/projectsveltos/libsveltos/lib/set"
)

var (
	setupLog             = ctrl.Log.WithName("setup")
	metricsAddr          string
	probeAddr            string
	workers              int
	concurrentReconciles int
	restConfigQPS        float32
	restConfigBurst      int
	webhookPort          int
	syncPeriod           time.Duration
)

const (
	defaultReconcilers = 10
	defaultWorkers     = 20
)

func main() {
	scheme, err := controllers.InitScheme()
	if err != nil {
		os.Exit(1)
	}

	klog.InitFlags(nil)

	initFlags(pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(klog.Background())

	ctrlOptions := ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port: webhookPort,
			}),
		Cache: cache.Options{
			SyncPeriod: &syncPeriod,
		},
	}

	restConfig := ctrl.GetConfigOrDie()
	restConfig.QPS = restConfigQPS
	restConfig.Burst = restConfigBurst

	mgr, err := ctrl.NewManager(restConfig, ctrlOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	var yttController controller.Controller
	yttReconciler := (&controllers.YttSourceReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		ReferenceMap:         make(map[corev1.ObjectReference]*libsveltosset.Set),
		YttSourceMap:         make(map[types.NamespacedName]*libsveltosset.Set),
		PolicyMux:            sync.Mutex{},
		ConcurrentReconciles: concurrentReconciles,
	})
	yttController, err = yttReconciler.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "YttSource")
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

	go fluxWatchers(ctx, mgr,
		yttReconciler, yttController,
		setupLog)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func initFlags(fs *pflag.FlagSet) {
	fs.StringVar(&metricsAddr,
		"metrics-bind-address",
		":8080",
		"The address the metric endpoint binds to.")

	fs.StringVar(&probeAddr,
		"health-probe-bind-address",
		":8081",
		"The address the probe endpoint binds to.")

	fs.IntVar(
		&workers,
		"worker-number",
		defaultWorkers,
		"Number of worker. Workers are used to deploy features in CAPI clusters")

	fs.IntVar(
		&concurrentReconciles,
		"concurrent-reconciles",
		defaultReconcilers,
		"concurrent reconciles is the maximum number of concurrent Reconciles which can be run. Defaults to 10")

	const defautlRestConfigQPS = 20
	fs.Float32Var(&restConfigQPS, "kube-api-qps", defautlRestConfigQPS,
		fmt.Sprintf("Maximum queries per second from the controller client to the Kubernetes API server. Defaults to %d",
			defautlRestConfigQPS))

	const defaultRestConfigBurst = 30
	fs.IntVar(&restConfigBurst, "kube-api-burst", defaultRestConfigBurst,
		fmt.Sprintf("Maximum number of queries that should be allowed in one burst from the controller client to the Kubernetes API server. Default %d",
			defaultRestConfigBurst))

	const defaultWebhookPort = 9443
	fs.IntVar(&webhookPort, "webhook-port", defaultWebhookPort,
		"Webhook Server port")

	const defaultSyncPeriod = 10
	fs.DurationVar(&syncPeriod, "sync-period", defaultSyncPeriod*time.Minute,
		fmt.Sprintf("The minimum interval at which watched resources are reconciled (e.g. 15m). Default: %d minutes",
			defaultSyncPeriod))
}

// fluxCRDHandler restarts process if a Flux CRD is updated
func fluxCRDHandler(gvk *schema.GroupVersionKind, action crd.ChangeType) {
	if action == crd.Modify {
		return
	}

	if gvk.Group == sourcev1.GroupVersion.Group {
		setupLog.V(logsettings.LogInfo).Info("Initiating graceful restart due to Flux CRD update",
			"GVK", gvk.String(), "Action", string(action))

		if killErr := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); killErr != nil {
			panic("kill -TERM failed")
		}
	}
}

// isFluxInstalled returns true if Flux is installed, false otherwise
func isFluxInstalled(ctx context.Context, c client.Client) (bool, error) {
	gitRepositoryCRD := &apiextensionsv1.CustomResourceDefinition{}

	err := c.Get(ctx, types.NamespacedName{Name: "gitrepositories.source.toolkit.fluxcd.io"},
		gitRepositoryCRD)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func fluxWatchers(ctx context.Context, mgr ctrl.Manager,
	yttSourceReconciler *controllers.YttSourceReconciler, yttSourceController controller.Controller,
	logger logr.Logger) {

	const maxRetries = 20
	retries := 0
	for {
		fluxPresent, err := isFluxInstalled(ctx, mgr.GetClient())
		if err != nil {
			if retries < maxRetries {
				logger.Info(fmt.Sprintf("failed to verify if Flux is present: %v", err))
				time.Sleep(time.Second)
			}
			retries++
		} else {
			if !fluxPresent {
				setupLog.V(logsettings.LogInfo).Info("Flux currently not present. Starting CRD watcher")
				go crd.WatchCustomResourceDefinition(ctx, mgr.GetConfig(), fluxCRDHandler, setupLog)
			} else {
				setupLog.V(logsettings.LogInfo).Info("Flux present.")
				err = yttSourceReconciler.WatchForFlux(mgr, yttSourceController)
				if err != nil {
					continue
				}
			}
			return
		}
	}
}
