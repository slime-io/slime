/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package bootstrap

import (
	"istio.io/libistio/galley/pkg/config/util/kuberesource"
	"istio.io/libistio/pkg/config/schema/snapshots"
	"istio.io/pkg/env"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"slime.io/slime/modules/meshregistry/pkg/features"
	"slime.io/slime/modules/meshregistry/pkg/util"
	"time"
)

const (
	defaultMeshConfigFolder = "/etc/mesh-config/"
	defaultMeshConfigFile   = defaultMeshConfigFolder + "mesh"
)

var podNamespace = env.RegisterStringVar("POD_NAMESPACE", "istio-system", "").Get()

type Args struct {
	// Path to the mesh config file
	MeshConfigFile    string
	EnableGRPCTracing bool
	WatchedNamespaces string
	// Resync period for rescanning Kubernetes resources
	ResyncPeriod util.Duration

	// Enable service discovery / endpoint processing.
	EnableServiceDiscovery bool

	// ExcludedResourceKinds is a list of resource kinds for which no source events will be triggered.
	// DEPRECATED
	ExcludedResourceKinds []string

	Snapshots []string
}

// DefaultArgs allocates an Args struct initialized with Galley's default configuration.
func DefaultArgs() *Args {
	return &Args{
		ResyncPeriod:          0,
		MeshConfigFile:        defaultMeshConfigFile,
		ExcludedResourceKinds: kuberesource.DefaultExcludedResourceKinds(),
		Snapshots:             []string{snapshots.Default},
	}
}

type BusinessArgs struct {
	RegionLabels, ZoneLabels, SubzoneLabels []string // TODO support set via args
}

type RegistryArgs struct {
	Args

	Business BusinessArgs

	Mcp McpArgs
	K8S K8SArgs

	K8SSource       K8SSourceArgs
	ZookeeperSource ZookeeperSourceArgs
	EurekaSource    EurekaSourceArgs
	NacosSource     NacosSourceArgs

	HTTPServerAddr string
	// istio revision
	Revision string
	// istio revision match for crds
	RevCrds            string
	RegistryStartDelay util.Duration
}

type SourceArgs struct {
	// enable the source
	Enabled bool
	// ready time to wait, non-0 means optional
	WaitTime util.Duration
	// Set refresh period. meaningful for sources which support and is in `polling` mode
	RefreshPeriod util.Duration
	GatewayModel  bool
	// patch instances label
	LabelPatch bool
	// svc port for services
	SvcPort uint32
	// if empty, those endpoints with ns attr will be aggregated into a no-ns service like "foo"
	DefaultServiceNs string
	ResourceNs       string
}

type K8SSourceArgs struct {
	SourceArgs

	WatchedNamespaces string

	// Enables extra k8s file source.
	EnableConfigFile bool
	// path of k8s file source
	ConfigPath string
	// WatchConfigFiles if set to true, enables Fsnotify watcher for watching and signaling config file changes.
	// Default is false
	WatchConfigFiles bool
}

type ZookeeperSourceArgs struct {
	SourceArgs

	Address []string
	// ignore label in ZookeeperSource instance
	IgnoreLabel       []string
	ConnectionTimeout util.Duration
	// dubbo register node in Zookeeper
	RegistryRootNode            string
	ApplicationRegisterRootNode string
	// zk mode for get zk info
	Mode string

	// dubbo configs

	// whether to gen dubbo `Sidecar`
	EnableDubboSidecar bool
	// the removed dep service of an app will only be effective when so much time has passed (since last)
	TrimDubboRemoveDepInterval util.Duration
	// specify how to map `app` to label key:value pair
	DubboWorkloadAppLabel string

	// mcp configs
}

type EurekaSourceArgs struct {
	SourceArgs

	Address []string
	// EurekaSource address belongs to nsf or not
	NsfEureka bool
	// need k8sDomainSuffix in Host
	K8sDomainSuffix bool
	// need ns in Host
	NsHost bool
}

type NacosSourceArgs struct {
	SourceArgs

	Address []string
	// nacos mode for get nacos info
	Mode string
	// namespace value for nacos client
	Namespace string
	// group value for nacos client
	Group string
	// nacos service name is like name.ns
	NameWithNs bool
	// need k8sDomainSuffix in Host
	K8sDomainSuffix bool
	// need ns in Host
	NsHost bool
}

type McpArgs struct {
	ServerUrl string
	// Enables the use of resource version in annotations.
	EnableAnnoResVer bool
	// Enables incremental push.
	EnableIncPush bool
	// non-0 means enable clean zombie config brought by incremental push.
	CleanZombieInterval util.Duration
}

type K8SArgs struct {
	// the ID of the cluster in which this mesh-registry instance is deployed
	ClusterID string
	// Select a namespace where the multicluster controller resides. If not set, uses ${POD_NAMESPACE} environment variable
	ClusterRegistriesNamespace string

	ApiServerUrl string
	// specify api server url to get deploy or pod info
	ApiServerUrlForDeploy string

	// KubeRestConfig has a rest config, common with other components
	KubeRestConfig *rest.Config `json:"-"`

	// The path to kube configuration file.
	// Use a Kubernetes configuration file instead of in-cluster configuration
	KubeConfig string
}

func NewRegistryArgs() *RegistryArgs {
	a := *DefaultArgs()
	return &RegistryArgs{
		Args:    a,
		RevCrds: "sidecars,destinationrules,envoyfilters,gateways,virtualservices",
		Mcp: McpArgs{
			ServerUrl:           "xds://0.0.0.0:16010",
			EnableAnnoResVer:    true,
			EnableIncPush:       true,
			CleanZombieInterval: 0,
		},
		K8S: K8SArgs{
			ClusterID:                  features.ClusterName,
			ClusterRegistriesNamespace: podNamespace,
		},
		RegistryStartDelay: util.Duration(5 * time.Second),

		K8SSource: K8SSourceArgs{
			SourceArgs:        SourceArgs{},
			EnableConfigFile:  false,
			WatchedNamespaces: metav1.NamespaceAll,
		},
		ZookeeperSource: ZookeeperSourceArgs{
			SourceArgs: SourceArgs{
				RefreshPeriod: util.Duration(10 * time.Second),
				LabelPatch:    true,
				ResourceNs:    "dubbo",
			},
			IgnoreLabel:                 []string{"pid", "timestamp", "dubbo"},
			Mode:                        "polling",
			ConnectionTimeout:           util.Duration(30 * time.Second),
			RegistryRootNode:            "/dubbo",
			ApplicationRegisterRootNode: "/services",
			TrimDubboRemoveDepInterval:  util.Duration(24 * time.Hour),
			EnableDubboSidecar:          true,
			DubboWorkloadAppLabel:       "app",
		},
		EurekaSource: EurekaSourceArgs{
			SourceArgs: SourceArgs{
				RefreshPeriod: util.Duration(30 * time.Second),
				LabelPatch:    true,
				SvcPort:       80,
				// should set it to "xx" explicitly to get the same behaviour as before("foo.eureka")
				DefaultServiceNs: "",
				ResourceNs:       "eureka",
			},
			K8sDomainSuffix: true,
			NsHost:          true,
		},
		NacosSource: NacosSourceArgs{
			SourceArgs: SourceArgs{
				RefreshPeriod: util.Duration(30 * time.Second),
				LabelPatch:    true,
				SvcPort:       80,
			},
			Mode:            "watching",
			K8sDomainSuffix: true,
			NsHost:          true,
		},
	}
}
