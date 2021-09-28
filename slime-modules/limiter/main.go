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
	"os"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	ctrl "sigs.k8s.io/controller-runtime"
	istioapi "slime.io/slime/slime-framework/apis"
	"slime.io/slime/slime-framework/bootstrap"
	istiocontroller "slime.io/slime/slime-framework/controllers"
	"slime.io/slime/slime-framework/util"
	microserviceslimeiov1alpha1 "slime.io/slime/slime-modules/limiter/api/v1alpha1"
	"slime.io/slime/slime-modules/limiter/controllers"
	// +kubebuilder:scaffold:imports
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = microserviceslimeiov1alpha1.AddToScheme(scheme)
	_ = istioapi.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	config := bootstrap.GetModuleConfig()
	err := util.InitLog(config.Global.Log.LogLevel, config.Global.Log.KlogLevel)
	if err != nil {
		panic(err.Error())
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: config.Global.Misc["metrics-addr"],
		Port:               9443,
		LeaderElection:     config.Global.Misc["enable-leader-election"] == "on",
		LeaderElectionID:   "limiter",
	})
	if err != nil {
		log.Errorf("unable to start manager, %+v", err)
		os.Exit(1)
	}
	env := bootstrap.Environment{}
	env.Config = config
	client, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		log.Errorf("unable to start manager, %+v", err)
		os.Exit(1)
	}
	env.K8SClient = client
	rec := controllers.NewReconciler(mgr, &env)
	if err = rec.SetupWithManager(mgr); err != nil {
		log.Errorf("unable to create controller SmartLimiter, %+v", err)
		os.Exit(1)
	}

	// add dr reconcile
	if err = (&istiocontroller.DestinationRuleReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Errorf("unable to create controller DestinationRule, %+v", err)
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	go bootstrap.AuxiliaryHttpServerStart(config.Global.Misc["aux-addr"])

	log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Errorf("problem running manager, %+v", err)
		os.Exit(1)
	}
}
