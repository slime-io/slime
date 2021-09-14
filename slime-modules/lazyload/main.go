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

	uberzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	istioapi "slime.io/slime/slime-framework/apis"
	"slime.io/slime/slime-framework/bootstrap"
	basecontroller "slime.io/slime/slime-framework/controllers"
	"slime.io/slime/slime-framework/util"
	microserviceslimeiov1alpha1 "slime.io/slime/slime-modules/lazyload/api/v1alpha1"
	"slime.io/slime/slime-modules/lazyload/controllers"
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

	// TODO - add pause/resume logic for module
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()
	encoderCfg := uberzap.NewDevelopmentEncoderConfig()
	encoderCfg.EncodeTime = util.TimeEncoder
	ctrl.SetLogger(zap.New(zap.UseDevMode(true), zap.Encoder(zapcore.NewConsoleEncoder(encoderCfg))))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "plugin",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	env := bootstrap.Environment{}
	env.Config = bootstrap.GetModuleConfig()
	client, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "create a new clientSet failed")
		os.Exit(1)
	}
	env.K8SClient = client

	sfReconciler := controllers.NewReconciler(mgr, &env)

	var builder basecontroller.ObjectReconcilerBuilder
	if err := builder.Add(basecontroller.ObjectReconcileItem{
		Name: "ServiceFence",
		R:    sfReconciler,
	}).Add(basecontroller.ObjectReconcileItem{
		Name: "VirtualService",
		R: &basecontroller.VirtualServiceReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("VirtualService"),
			Scheme: mgr.GetScheme(),
		},
	}).Add(basecontroller.ObjectReconcileItem{
		Name:    "Service",
		ApiType: &corev1.Service{},
		R:       reconcile.Func(sfReconciler.ReconcileService),
	}).Add(basecontroller.ObjectReconcileItem{
		Name:    "Namespace",
		ApiType: &corev1.Namespace{},
		R:       reconcile.Func(sfReconciler.ReconcileNamespace),
	}).Build(mgr); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	go bootstrap.HealthCheckStart()
	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
