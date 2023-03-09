/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package bootstrap

import (
	"errors"
	"time"

	"istio.io/libistio/galley/pkg/config/util/kuberesource"
	"istio.io/libistio/pkg/config/schema/snapshots"
	"istio.io/pkg/env"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"slime.io/slime/modules/meshregistry/pkg/features"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

const (
	defaultMeshConfigFolder = "/etc/mesh-config/"
	defaultMeshConfigFile   = defaultMeshConfigFolder + "mesh"
)

var podNamespace = env.RegisterStringVar("POD_NAMESPACE", "istio-system", "").Get()

type Args struct {
	// Path to the mesh config file
	MeshConfigFile string `json:"MeshConfigFile,omitempty"`

	EnableGRPCTracing bool   `json:"EnableGRPCTracing,omitempty"`
	WatchedNamespaces string `json:"WatchedNamespaces,omitempty"`
	// Resync period for rescanning Kubernetes resources
	ResyncPeriod util.Duration `json:"ResyncPeriod,omitempty"`

	// Enable service discovery / endpoint processing.
	EnableServiceDiscovery bool `json:"EnableServiceDiscovery,omitempty"`

	// ExcludedResourceKinds is a list of resource kinds for which no source events will be triggered.
	// DEPRECATED
	ExcludedResourceKinds []string `json:"ExcludedResourceKinds,omitempty"`

	Snapshots []string `json:"Snapshots,omitempty"`
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
	RegionLabels  []string `json:"RegionLabels,omitempty"`
	ZoneLabels    []string `json:"ZoneLabels,omitempty"`
	SubzoneLabels []string `json:"SubzoneLabels,omitempty"`
	// TODO support set via args
}

type RegistryArgs struct {
	Args

	Business BusinessArgs `json:"Business"`

	Mcp McpArgs `json:"Mcp"`
	K8S K8SArgs `json:"K8S"`

	K8SSource       K8SSourceArgs       `json:"K8SSource"`
	ZookeeperSource ZookeeperSourceArgs `json:"ZookeeperSource"`
	EurekaSource    EurekaSourceArgs    `json:"EurekaSource"`
	NacosSource     NacosSourceArgs     `json:"NacosSource"`

	HTTPServerAddr string `json:"HTTPServerAddr,omitempty"`
	// istio revision
	Revision string `json:"Revision,omitempty"`
	// istio revision match for crds
	RevCrds            string        `json:"RevCrds,omitempty"`
	RegistryStartDelay util.Duration `json:"RegistryStartDelay,omitempty"`
}

func (args *RegistryArgs) Validate() error {
	if err := args.ZookeeperSource.Validate(); err != nil {
		return err
	}
	if err := args.EurekaSource.Validate(); err != nil {
		return err
	}
	if err := args.NacosSource.Validate(); err != nil {
		return err
	}
	return nil
}

type SourceArgs struct {
	// enable the source
	Enabled bool `json:"Enabled,omitempty"`
	// ready time to wait, non-0 means optional
	WaitTime util.Duration `json:"WaitTime,omitempty"`
	// Set refresh period. meaningful for sources which support and is in `polling` mode
	RefreshPeriod util.Duration `json:"RefreshPeriod,omitempty"`
	GatewayModel  bool          `json:"GatewayModel,omitempty"`
	// patch instances label
	LabelPatch bool `json:"LabelPatch,omitempty"`
	// svc port for services
	SvcPort uint32 `json:"SvcPort,omitempty"`
	// if empty, those endpoints with ns attr will be aggregated into a no-ns service like "foo"
	DefaultServiceNs string `json:"DefaultServiceNs,omitempty"`
	ResourceNs       string `json:"ResourceNs,omitempty"`
	// A list of selectors that specify the set of service instances to be processed,
	// configured in the same way as the k8s label selector.
	EndpointSelectors []*metav1.LabelSelector `json:"EndpointSelectors,omitempty"`
	// Endpoint selectors for specific service, the key of the map is the service name
	ServicedEndpointSelectors map[string][]*metav1.LabelSelector `json:"ServicedEndpointSelectors,omitempty"`
}

type K8SSourceArgs struct {
	SourceArgs

	WatchedNamespaces string `json:"WatchedNamespaces,omitempty"`

	// Enables extra k8s file source.
	EnableConfigFile bool `json:"EnableConfigFile,omitempty"`
	// path of k8s file source
	ConfigPath string `json:"ConfigPath,omitempty"`
	// WatchConfigFiles if set to true, enables Fsnotify watcher for watching and signaling config file changes.
	// Default is false
	WatchConfigFiles bool `json:"WatchConfigFiles,omitempty"`
}

type ZookeeperSourceArgs struct {
	SourceArgs

	Address []string `json:"Address,omitempty"`
	// ignore label in ZookeeperSource instance
	IgnoreLabel       []string      `json:"IgnoreLabel,omitempty"`
	ConnectionTimeout util.Duration `json:"ConnectionTimeout,omitempty"`
	// dubbo register node in Zookeeper
	RegistryRootNode            string `json:"RegistryRootNode,omitempty"`
	ApplicationRegisterRootNode string `json:"ApplicationRegisterRootNode,omitempty"`
	// zk mode for get zk info
	Mode                string `json:"Mode,omitempty"`
	WatchingWorkerCount int    `json:"WatchingWorkerCount,omitempty"`

	// dubbo configs

	// whether to gen dubbo `Sidecar`
	EnableDubboSidecar bool `json:"EnableDubboSidecar,omitempty"`
	// the removed dep service of an app will only be effective when so much time has passed (since last)
	TrimDubboRemoveDepInterval util.Duration `json:"TrimDubboRemoveDepInterval,omitempty"`
	// specify how to map `app` to label key:value pair
	DubboWorkloadAppLabel string `json:"DubboWorkloadAppLabel,omitempty"`

	// mcp configs
}

func (zkArgs *ZookeeperSourceArgs) Validate() error {
	if !zkArgs.Enabled {
		return nil
	}
	if len(zkArgs.Address) == 0 {
		return errors.New("zookeeper server address must be set when zookeeper source is enabled")
	}
	return nil
}

type EurekaSourceArgs struct {
	SourceArgs

	Address []string `json:"Address,omitempty"`
	// EurekaSource address belongs to nsf or not
	NsfEureka bool `json:"NsfEureka,omitempty"`
	// need k8sDomainSuffix in Host
	K8sDomainSuffix bool `json:"K8SDomainSuffix,omitempty"`
	// need ns in Host
	NsHost bool `json:"NsHost,omitempty"`
}

func (eurekaArgs *EurekaSourceArgs) Validate() error {
	if !eurekaArgs.Enabled {
		return nil
	}
	if len(eurekaArgs.Address) == 0 {
		return errors.New("eureka server address must be set when eureka source is enabled")
	}
	return nil
}

type NacosSourceArgs struct {
	SourceArgs

	Address []string `json:"Address,omitempty"`
	// nacos mode for get nacos info
	Mode string `json:"Mode,omitempty"`
	// namespace value for nacos client
	Namespace string `json:"Namespace,omitempty"`
	// group value for nacos client
	Group string `json:"Group,omitempty"`
	// nacos service name is like name.ns
	NameWithNs bool `json:"NameWithNs,omitempty"`
	// need k8sDomainSuffix in Host
	K8sDomainSuffix bool `json:"K8SDomainSuffix,omitempty"`
	// need ns in Host
	NsHost bool `json:"NsHost,omitempty"`
	// username and password for nacos auth
	Username string `json:"Username,omitempty"`
	Password string `json:"Password,omitempty"`
	// fetch services from all namespaces, only support Polling mode
	AllNamespaces bool `json:"AllNamespaces,omitempty"`
	//  If set, namespace and group information will be injected into the ep's metadata using the set key.
	MetaKeyGroup     string `json:"MetaKeyGroup,omitempty"`
	MetaKeyNamespace string `json:"MetaKeyNamespace,omitempty"`
}

func (nacosArgs *NacosSourceArgs) Validate() error {
	if !nacosArgs.Enabled {
		return nil
	}
	if len(nacosArgs.Address) == 0 {
		return errors.New("nacos server address must be set when nacos source is enabled")
	}
	return nil
}

type McpArgs struct {
	ServerUrl string `json:"ServerUrl,omitempty"`
	// Enables the use of resource version in annotations.
	EnableAnnoResVer bool `json:"EnableAnnoResVer,omitempty"`
	// Enables incremental push.
	EnableIncPush bool `json:"EnableIncPush,omitempty"`
	// non-0 means enable clean zombie config brought by incremental push.
	CleanZombieInterval util.Duration `json:"CleanZombieInterval,omitempty"`
}

type K8SArgs struct {
	// the ID of the cluster in which this mesh-registry instance is deployed
	ClusterID string `json:"ClusterID,omitempty"`
	// Select a namespace where the multicluster controller resides. If not set, uses ${POD_NAMESPACE} environment variable
	ClusterRegistriesNamespace string `json:"ClusterRegistriesNamespace,omitempty"`

	ApiServerUrl string `json:"ApiServerUrl,omitempty"`
	// specify api server url to get deploy or pod info
	ApiServerUrlForDeploy string `json:"ApiServerUrlForDeploy,omitempty"`

	// KubeRestConfig has a rest config, common with other components
	KubeRestConfig *rest.Config `json:"-"`

	// The path to kube configuration file.
	// Use a Kubernetes configuration file instead of in-cluster configuration
	KubeConfig string `json:"KubeConfig,omitempty"`
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
			WatchingWorkerCount:         10,
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
				RefreshPeriod:    util.Duration(30 * time.Second),
				LabelPatch:       true,
				SvcPort:          80,
				DefaultServiceNs: "",
				ResourceNs:       "nacos",
			},
			Mode:            "watching",
			K8sDomainSuffix: true,
			NsHost:          true,
		},
	}
}
