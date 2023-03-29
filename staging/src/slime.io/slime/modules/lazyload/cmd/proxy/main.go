package main

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"net"
	"net/http"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"slime.io/slime/modules/lazyload/pkg/proxy"
)

const (
	EnvProbePort         = "PROBE_PORT"
	EnvLogLevel          = "LOG_LEVEL"
	EnvPodNamespace      = "POD_NAMESPACE"
	DisableSvcController = "DISABLE_SVC_CONTROLLER"
)

var (
	serverMutex sync.RWMutex
	servers     = map[int]*http.Server{}

	probePort = os.Getenv(EnvProbePort)
	logLevel  = os.Getenv(EnvLogLevel)

	disableSvcController = os.Getenv(DisableSvcController) == "true"

	configLabelSelector = "lazyload.slime.io/config=global-sidecar"

	Cache *proxy.Cache
)

func init() {
	if !disableSvcController {
		Cache = &proxy.Cache{
			Data: make(map[types.NamespacedName]struct{}),
		}
	}
}

func main() {

	// set log config
	if logLevel == "" {
		logLevel = "info"
	}
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		os.Exit(1)
	}
	log.SetLevel(level)
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.RFC3339,
	})

	if probePort != "" {
		// start health check server
		go func() {
			handler := &proxy.HealthzProxy{}
			log.Println("Starting health check on", ":"+probePort)
			if err := http.ListenAndServe(":"+probePort, handler); err != nil {
				log.Fatal("ListenAndServe:", err)
			}
		}()
	}

	ctx := ctrl.SetupSignalHandler()

	if !disableSvcController {
		if err := startSvcCache(ctx); err != nil {
			log.Fatal(err)
		}
		log.Infof("start svc cache")
	}

	controller, err := newConfigMapController()
	if err != nil {
		log.Fatal(err)
	}
	controller.Run(ctx.Done())
	stopListenAndServe()
}

func newConfigMapController() (*controller, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	lw := cache.NewFilteredListWatchFromClient(client.CoreV1().RESTClient(), "configmaps", os.Getenv(EnvPodNamespace), func(options *metav1.ListOptions) {
		options.LabelSelector = configLabelSelector
	})
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	indexer, informer := cache.NewIndexerInformer(lw, &corev1.ConfigMap{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
	}, cache.Indexers{})
	return NewController(indexer, queue, informer), nil
}

func stopListenAndServe() {
	serverMutex.Lock()
	defer serverMutex.Unlock()
	var wg sync.WaitGroup
	for _, srv := range servers {
		wg.Add(1)
		go func(s *http.Server) {
			shutdownServer(s)
			wg.Done()
		}(srv)
	}
	wg.Wait()
	log.Infof("Shutdown all proxy server")
}

func startListenAndServe(wormholePorts map[int]struct{}) {
	if wormholePorts == nil {
		wormholePorts = make(map[int]struct{})
	}
	// At least we listen and serve at "80"
	wormholePorts[80] = struct{}{}

	serverMutex.Lock()
	defer serverMutex.Unlock()
	log.Infof("Starting listen and serve with wormholePorts: %v", wormholePorts)
	for whPort := range wormholePorts {
		whStrPort := strconv.Itoa(whPort)
		if whStrPort == probePort {
			log.Warnf("ProbePort is conflict with wormholePort %v, skip", whStrPort)
			continue
		}
		if _, exist := servers[whPort]; !exist {
			srv := &http.Server{
				Addr: "0.0.0.0" + ":" + strconv.Itoa(whPort),
				Handler: &proxy.Proxy{
					WormholePort: whPort,
					SvcCache:     Cache,
				},
			}
			servers[whPort] = srv
			go startServer(srv)
		}
	}

	// ports will only be automatically increased when auto managing.
	// more info can be found at: https://github.com/slime-io/slime/pull/157
	// for whPort, srv := range servers {
	// 	if _, exist := wormholePorts[whPort]; !exist {
	// 		delete(servers, whPort)
	// 		go shutdownServer(srv)
	// 	}
	// }
}

func startServer(srv *http.Server) {
	log.Infof("Starting proxy on: %s", srv.Addr)
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
		},
	}
	l, err := lc.Listen(context.Background(), "tcp", srv.Addr)
	if err != nil {
		log.Warn("Proxy Listen error:", err)
	} else {
		if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
			log.Warn("Proxy Serve error:", err)
		}
	}
}

func shutdownServer(srv *http.Server) error {
	if srv == nil {
		return nil
	}
	log.Infof("Stopting proxy on: %s", srv.Addr)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
