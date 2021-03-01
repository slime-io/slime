package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"

	"slime.io/slime/pkg/apis"
	"slime.io/slime/pkg/apis/config/v1alpha1"
	"slime.io/slime/pkg/bootstrap"
	"slime.io/slime/pkg/controller"
	"slime.io/slime/pkg/controller/destinationrule"
	"slime.io/slime/pkg/controller/envoyplugin"
	"slime.io/slime/pkg/controller/pluginmanager"
	"slime.io/slime/pkg/controller/servicefence"
	"slime.io/slime/pkg/controller/smartlimiter"
	"slime.io/slime/pkg/controller/virtualservice"
	"slime.io/slime/version"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	kubemetrics "github.com/operator-framework/operator-sdk/pkg/kube-metrics"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/operator-framework/operator-sdk/pkg/metrics"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost               = "0.0.0.0"
	metricsPort         int32 = 8383
	operatorMetricsPort int32 = 8686
)
var log = logf.Log.WithName("cmd")

func printVersion() {
	log.Info(fmt.Sprintf("Operator Version: %s", version.Version))
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
}

func main() {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()

	// Use a zap logr.Logger implementation. If none of the zap
	// flags are configured (or if the zap flag set is not being
	// used), this defaults to a production zap logger.
	//
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.Logger())

	printVersion()

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		log.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	env := bootstrap.Environment{}
	env.Config = bootstrap.GetModuleConfig()
	fmt.Printf("%v\n", env.Config)

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	ctx := context.TODO()

	if env.Config.Plugin != nil && env.Config.Plugin.Enable {
		// Become the leader before proceeding
		err = leader.Become(ctx, "slime-plugin-lock")
		if err != nil {
			log.Error(err, "")
			os.Exit(1)
		}
	}

	if env.Config.Fence != nil && env.Config.Fence.Enable {
		err = leader.Become(ctx, "slime-fence-lock")
		if err != nil {
			log.Error(err, "")
			os.Exit(1)
		}
	}

	if env.Config.Limiter != nil && env.Config.Limiter.Enable {
		err = leader.Become(ctx, "slime-limiter-lock")
		if err != nil {
			log.Error(err, "")
			os.Exit(1)
		}
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		Namespace:          "",
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
	})

	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	client, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}
	env.K8SClient = client

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	for _, v := range processConfig(env.Config) {
		controller.AddToManagerFuncs = append(controller.AddToManagerFuncs, v)
	}

	stop := make(chan struct{})
	// Setup all Controllers
	if err := controller.AddToManager(mgr, &env); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Add the Metrics Service
	addMetrics(ctx, cfg, namespace)

	log.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		stop <- struct{}{}
		os.Exit(1)
	}
}

// addMetrics will create the Services and Service Monitors to allow the operator export the metrics by using
// the Prometheus operator
func addMetrics(ctx context.Context, cfg *rest.Config, namespace string) {
	if err := serveCRMetrics(cfg); err != nil {
		if errors.Is(err, k8sutil.ErrRunLocal) {
			log.Info("Skipping CR metrics server creation; not running in a cluster.")
			return
		}
		log.Info("Could not generate and serve custom resource metrics", "error", err.Error())
	}

	// Add to the below struct any other metrics ports you want to expose.
	servicePorts := []v1.ServicePort{
		{Port: metricsPort, Name: metrics.OperatorPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: metricsPort}},
		{Port: operatorMetricsPort, Name: metrics.CRPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: operatorMetricsPort}},
	}

	// Create Service object to expose the metrics port(s).
	service, err := metrics.CreateMetricsService(ctx, cfg, servicePorts)
	if err != nil {
		log.Info("Could not create metrics Service", "error", err.Error())
	}

	// CreateServiceMonitors will automatically create the prometheus-operator ServiceMonitor resources
	// necessary to configure Prometheus to scrape metrics from this operator.
	services := []*v1.Service{service}
	_, err = metrics.CreateServiceMonitors(cfg, namespace, services)
	if err != nil {
		log.Info("Could not create ServiceMonitor object", "error", err.Error())
		// If this operator is deployed to a cluster without the prometheus-operator running, it will return
		// ErrServiceMonitorNotPresent, which can be used to safely skip ServiceMonitor creation.
		if err == metrics.ErrServiceMonitorNotPresent {
			log.Info("Install prometheus-operator in your cluster to create ServiceMonitor objects", "error", err.Error())
		}
	}
}

// serveCRMetrics gets the Operator/CustomResource GVKs and generates metrics based on those types.
// It serves those metrics on "http://metricsHost:operatorMetricsPort".
func serveCRMetrics(cfg *rest.Config) error {
	// Below function returns filtered operator/CustomResource specific GVKs.
	// For more control override the below GVK list with your own custom logic.
	filteredGVK, err := k8sutil.GetGVKsFromAddToScheme(apis.AddToScheme)
	if err != nil {
		return err
	}
	// Get the namespace the operator is currently deployed in.
	operatorNs, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return err
	}
	// To generate metrics in other namespaces, add the values below.
	ns := []string{operatorNs}
	// Generate and serve custom resource specific metrics.
	err = kubemetrics.GenerateAndServeCRMetrics(cfg, ns, filteredGVK, metricsHost, operatorMetricsPort)
	if err != nil {
		return err
	}
	return nil
}

func processConfig(Config *v1alpha1.Config) map[controller.Collection]func(manager.Manager, *bootstrap.Environment) error {

	f := make(map[controller.Collection]func(manager.Manager, *bootstrap.Environment) error)
	if Config.Limiter != nil && Config.Limiter.Enable {
		controller.UpdateHook[controller.SmartLimiter] = []func(object v12.Object, args ...interface{}) error{smartlimiter.DoUpdate}
		controller.DeleteHook[controller.SmartLimiter] = []func(reconcile.Request, ...interface{}) error{smartlimiter.DoRemove}
		controller.UpdateHook[controller.DestinationRule] = []func(object v12.Object, args ...interface{}) error{destinationrule.DoUpdate}
		f[controller.SmartLimiter] = smartlimiter.Add
		f[controller.DestinationRule] = destinationrule.Add
	}

	if Config.Plugin != nil && Config.Plugin.Enable {
		controller.UpdateHook[controller.EnvoyPlugin] = []func(object v12.Object, args ...interface{}) error{envoyplugin.DoUpdate}
		controller.UpdateHook[controller.PluginManager] = []func(object v12.Object, args ...interface{}) error{pluginmanager.DoUpdate}
		f[controller.EnvoyPlugin] = envoyplugin.Add
		f[controller.PluginManager] = pluginmanager.Add
	}

	if Config.Fence != nil && Config.Fence.Enable {
		controller.UpdateHook[controller.ServiceFence] = []func(object v12.Object, args ...interface{}) error{servicefence.DoUpdate}
		controller.UpdateHook[controller.VirtualService] = []func(object v12.Object, args ...interface{}) error{virtualservice.ServiceFenceProcess}
		f[controller.ServiceFence] = servicefence.Add
		f[controller.VirtualService] = virtualservice.Add
	}

	return f
}
