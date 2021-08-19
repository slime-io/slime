/*


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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	istioapi "slime.io/slime/slime-framework/apis"
	"slime.io/slime/slime-framework/bootstrap"
	istiocontroller "slime.io/slime/slime-framework/controllers"
	microserviceslimeiov1alpha1 "slime.io/slime/slime-modules/limiter/api/v1alpha1"
	"slime.io/slime/slime-modules/limiter/controllers"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = microserviceslimeiov1alpha1.AddToScheme(scheme)
	_ = istioapi.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "limiter",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	env := bootstrap.Environment{}
	env.Config = bootstrap.GetModuleConfig()
	client, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		os.Exit(1)
	}
	env.K8SClient = client
	rec := controllers.NewReconciler(mgr, &env)
	if err = rec.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SmartLimiter")
		os.Exit(1)
	}

	// add dr reconcile
	if err = (&istiocontroller.DestinationRuleReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("DestinationRule"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DestinationRule")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	// TODO - Add ModuleHealthCheckRegister
	go bootstrap.HealthCheckStart()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
