/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package bootstrap

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"istio.io/libistio/pkg/config/schema/collections"
	"istio.io/libistio/pkg/env"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"slime.io/slime/modules/meshregistry/pkg/features"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

const (
	// zookeeper
	defaultConsumerPath = "/consumers"
	disableConsumerPath = "-"
)

var podNamespace = env.RegisterStringVar("POD_NAMESPACE", "istio-system", "").Get()

type Args struct {
	// Path to the mesh config file
	// UNSUPPORTED
	MeshConfigFile string `json:"MeshConfigFile,omitempty"`

	EnableGRPCTracing bool `json:"EnableGRPCTracing,omitempty"`
	// DEPRECATED, use K8SSource instead
	WatchedNamespaces string `json:"WatchedNamespaces,omitempty"`
	// Resync period for rescanning Kubernetes resources
	ResyncPeriod util.Duration `json:"ResyncPeriod,omitempty"`

	// Enable service discovery / endpoint processing.
	// Uselss for now, will be removed in the future.
	EnableServiceDiscovery bool `json:"EnableServiceDiscovery,omitempty"`

	// ExcludedResourceKinds is a list of resource kinds for which no source events will be triggered.
	// DEPRECATED, moved to K8SSource
	ExcludedResourceKinds []string `json:"ExcludedResourceKinds,omitempty"`

	// Snapshot is the name of the preset resource set, available values are `default` and `localAnalysis`,
	// default is `default`, which includes all pilot resources and k8s namesapce and service.
	// DEPRECATED, replaced by K8SSource.Collections
	Snapshots []string `json:"Snapshots,omitempty"`
}

// DefaultArgs allocates an Args struct initialized with Galley's default configuration.
func DefaultArgs() *Args {
	return &Args{
		ResyncPeriod:          0,
		ExcludedResourceKinds: collections.LegacyDefaultExcludeKubeResourceKinds(),
		Snapshots:             []string{CollectionsLegacyDefault},
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

	Business *BusinessArgs `json:"Business,omitempty"`

	Mcp *McpArgs `json:"Mcp,omitempty"`
	K8S *K8SArgs `json:"K8S,omitempty"`

	K8SSource       *K8SSourceArgs       `json:"K8SSource,omitempty"`
	ZookeeperSource *ZookeeperSourceArgs `json:"ZookeeperSource,omitempty"`
	EurekaSource    *EurekaSourceArgs    `json:"EurekaSource,omitempty"`
	NacosSource     *NacosSourceArgs     `json:"NacosSource,omitempty"`

	HTTPServerAddr string `json:"HTTPServerAddr,omitempty"`
	// istio revision
	Revision string `json:"Revision,omitempty"`
	// istio revision match for crds
	// UNSUPPORTED for now
	RevCrds            string        `json:"RevCrds,omitempty"`
	RegistryStartDelay util.Duration `json:"RegistryStartDelay,omitempty"`
}

// Validate validates the registry args. It should be called after the args are rectified.
func (args *RegistryArgs) Validate() error {
	if err := args.ZookeeperSource.Validate(); err != nil {
		return err
	}
	if err := args.EurekaSource.Validate(); err != nil {
		return err
	}
	if err := args.NacosSource.Validate(); err != nil { //nolint: revive
		return err
	}
	return nil
}

func (args *RegistryArgs) Rectify() *RegistryArgs {
	if args == nil {
		return args
	}
	args.ZookeeperSource.Rectify()
	args.NacosSource.Rectify()
	args.EurekaSource.Rectify()
	return args
}

type SourceArgs struct {
	// enable the source
	Enabled bool `json:"Enabled,omitempty"`
	// ready time to wait, non-0 means optional
	WaitTime util.Duration `json:"WaitTime,omitempty"`
	// Set refresh period. meaningful for sources which support and is in `polling` mode
	RefreshPeriod util.Duration `json:"RefreshPeriod,omitempty"`
	// Deprecated
	// almost equals `InstancePortAsSvcPort = false and K8sDomainSuffix = false` and will override them if true
	// NOTICE: does not support hot-reload
	GatewayModel bool `json:"GatewayModel,omitempty"`
	// patch instances label
	LabelPatch bool `json:"LabelPatch,omitempty"`
	// GenericProtocol is used to specify weather using generic protocol as the first protocol in the service entry,
	// default is false. If true, the final port protocol will be `GENERIC`, and the port name prefix will be
	// `generic-lower(SvcProtocol)`, e.g. `generic-dubbo`.
	GenericProtocol bool `json:"GenericProtocol,omitempty"`
	// svc protocol for services, for Eureka and Nacos, default is "HTTP", for Zookeeper, default is "DUBBO"
	SvcProtocol string `json:"SvcProtocol,omitempty"`
	// svc port for services, 0 will be ignored
	SvcPort               uint32 `json:"SvcPort,omitempty"`               // XXX
	InstancePortAsSvcPort bool   `json:"InstancePortAsSvcPort,omitempty"` // TODO
	// if empty, those endpoints with ns attr will be aggregated into a no-ns service like "foo"
	DefaultServiceNs string `json:"DefaultServiceNs,omitempty"`
	ResourceNs       string `json:"ResourceNs,omitempty"`

	// A list of selectors that specify the set of service instances to be processed.
	// The selectors are ORed together.
	EndpointSelectors []*EndpointSelector `json:"EndpointSelectors,omitempty"`
	// Endpoint selectors for specific service, the key of the map is the service name.
	// The selectors of each service are ORed together.
	// If the service matches ServicedEndpointSelector, the source scoped EndpointSelectors should be ignored,
	// unless AlwaysUseSourceScopedEpSelectors is set to true.
	ServicedEndpointSelectors map[string][]*EndpointSelector `json:"ServicedEndpointSelectors,omitempty"`
	// EmptyEpSelectorsExcludeAll is set to true and no ep selectors are configured, the source should exclude all eps.
	EmptyEpSelectorsExcludeAll bool `json:"EmptyEpSelectorsExcludeAll,omitempty"`
	// AlwaysUseSourceScopedEpSelectors if set to true, the source scoped EndpointSelectors should be processed
	// even if the service matches ServicedEndpointSelector
	AlwaysUseSourceScopedEpSelectors bool `json:"AlwaysUseSourceScopedEpSelectors,omitempty"`

	// MockService used to aggregate all the port of the services
	// MockServiceName is the host of the mock service
	MockServiceName string `json:"MockServiceName,omitempty"`
	// MockServiceEntryName is the name of the converted ServiceEntry
	MockServiceEntryName string `json:"MockServiceEntryName,omitempty"`
	// MockServiceMergeInstancePort and MockServiceMergeServicePort are used to specify whether to
	// merge the instance port and service port into the mock service port, at least one of them should be true.
	MockServiceMergeInstancePort bool `json:"MockServiceMergeInstancePort,omitempty"`
	MockServiceMergeServicePort  bool `json:"MockServiceMergeServicePort,omitempty"`

	// ServiceNaming is used to reassign the service to which the instance belongs
	ServiceNaming *ServiceNameConverter `json:"ServiceNaming,omitempty"`
	// ServiceHostAliases allows configuring additional aliases for the specified service host
	ServiceHostAliases []*ServiceHostAlias `json:"ServiceHostAliases,omitempty"`
	// ServiceAdditionalMetas allows configuring additional metadata for the specified service
	// when converting to a ServiceEntry.
	ServiceAdditionalMetas map[string]*MetadataWrapper `json:"ServiceAdditionalMetas,omitempty"`
	// InstanceMetaRelabel is used to adjust the metadata of the instance.
	// Note that ServiceNaming may refer to instance metadata, the InstanceMetaRelabel needs
	// to be processed before ServiceNaming.
	InstanceMetaRelabel *InstanceMetaRelabel `json:"InstanceMetaRelabel,omitempty"`

	// EnableEmptyProtection if set to true, the source should ignore conversion of the service with no endpoints,
	// which means the last conversion result with endpoints will be kept until the source gets the new endpoints.
	EnableEmptyProtection bool `json:"EnableEmptyProtection,omitempty"`
}

// IPRanges defines a set of ip with ip list and cidr list
type IPRanges struct {
	IPs   []string `json:"IPs,omitempty"`
	CIDRs []string `json:"CIDRs,omitempty"`
}

// EndpointSelector specifies which endpoints should be processed.
// Currently, endpoints are specified by the label and ip of the instance.
// The labelselector is the same as the label selector of k8s.
// The exclude ip ranges is used to exclude endpoints with the specified ip in the ip ranges.
// The label selector and exclude ip selector are ANDed.
type EndpointSelector struct {
	*metav1.LabelSelector
	ExcludeIPRanges *IPRanges `json:"ExcludeIPRanges,omitempty"`
}

type ServiceSelector struct {
	*metav1.LabelSelector
	// Invert the match result. Turns the selector into a blacklist.
	Invert bool `json:"Invert,omitempty"`
}

type MetadataWrapper struct {
	Annotations map[string]string `json:"Annotations,omitempty"`
	Labels      map[string]string `json:"Labels,omitempty"`
}

// ServiceHostAlias includes the original host and all aliases of the original host
type ServiceHostAlias struct {
	Host    string   `json:"Host,omitempty"`
	Aliases []string `json:"Aliases,omitempty"`
}

// ServiceNameConverter configures the service name of an instance,
// using Seq to connect the substring configured by each item.
type ServiceNameConverter struct {
	Sep   string              `json:"Sep,omitempty"`
	Items []ServiceNamingItem `json:"Items,omitempty"`
}

type ServiceNameItemKind string

var (
	InstanceBasicInfoKind ServiceNameItemKind = "$"
	InstanceMetadataKind  ServiceNameItemKind = "meta"
	StaticKind            ServiceNameItemKind = "static"

	InstanceBasicInfoSvc  = "service"
	InstanceBasicInfoIP   = "ip"
	InstanceBasicInfoPort = "port"
)

// ServiceNamingItem configure how a service name substring is generated.
// The Kind field indicates the data source of the substring and
// configurable values are `$`, `static` and `meta`.
//   - `$`: basic information of the instance, supports `service`(the original service name of the instance),
//     `ip`(the instance ip), `port`(the instance port).
//   - `meta`: metadata of the instance, the value of the Value field is the extracted key specified in the metadata.
//   - `static`: static configuration, directly using the value of the Value field.
type ServiceNamingItem struct {
	Kind  ServiceNameItemKind `json:"Kind,omitempty"`
	Value string              `json:"Value,omitempty"`
}

// InstanceMetaRelabel is used to configure how to adjust the metadata of the instance.
type InstanceMetaRelabel struct {
	// Items is the InstanceMetaRelabelItem configuration list, which is executed sequentially,
	// which means that the subsequent items will be processed on the results of the previous items.
	Items []*InstanceMetaRelabelItem `json:"Items,omitempty"`
}

// InstanceMetaRelabelItem represents an item used for relabeling instance metadata.
type InstanceMetaRelabelItem struct {
	// The key that currently exists in the instance metadata.
	Key string `json:"Key,omitempty"`
	// TargetKey is the new key to be added to the instance metadata based on the original key.
	TargetKey string `json:"TargetKey,omitempty"`
	// Whether to overwrite the value of the TargetKey if it already exists in the instance metadata.
	Overwrite bool `json:"Overwrite,omitempty"`
	// If the CreatedWithValue is not empty and the Key is not found in the instance metadata,
	// the TargetKey will be set to the CreatedWithValue value.
	CreatedWithValue string `json:"CreatedWithValue,omitempty"`
	// ValuesMapping is a map that associates values of the Key to values of the TargetKey.
	// If the Key's value is found in the map, the corresponding value is used for the TargetKey.
	// If not, the original value is used for the TargetKey.
	ValuesMapping map[string]string `json:"ValuesMapping,omitempty"`
}

func (args *SourceArgs) Validate() error {
	if args.MockServiceEntryName != "" && args.MockServiceName == "" {
		return fmt.Errorf("args MockServiceName empty but MockServiceEntryName %s", args.MockServiceEntryName)
	}

	if !args.InstancePortAsSvcPort && args.SvcPort == 0 {
		return fmt.Errorf("SvcPort == 0 while InstancePortAsSvcPort false is not permitted")
	}
	return nil
}

const (
	CollectionsAll           = "all"
	CollectionsIstio         = "istio"
	CollectionsLegacyDefault = "default"
	CollectionsLegacyLocal   = "localAnalysis"
)

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

	// Collections is the name of the preset resource set, available values are:
	//   - all: all resources used by istio
	//   - istio: all pilot and k8s gateway resources
	//   - default: all legacy `default` snapshot resources
	//   - localAnalysis: all legacy `localAnalysis` snapshot resources
	Collections []string `json:"Collections,omitempty"`
	// ExcludedResourceKinds is a list of resource kinds for which no source events will be triggered.
	ExcludedResourceKinds []string `json:"ExcludedResourceKinds,omitempty"`
}

type ZookeeperSourceArgs struct {
	SourceArgs

	Address []string `json:"Address,omitempty"`
	// ignore label in ZookeeperSource instance
	IgnoreLabel       []string      `json:"IgnoreLabel,omitempty"`
	ConnectionTimeout util.Duration `json:"ConnectionTimeout,omitempty"`
	// dubbo register node in Zookeeper
	RegistryRootNode string `json:"RegistryRootNode,omitempty"`
	// NOSUPPORTED
	ApplicationRegisterRootNode string `json:"ApplicationRegisterRootNode,omitempty"`
	// zk mode for get zk info
	Mode                string            `json:"Mode,omitempty"`
	WatchingWorkerCount int               `json:"WatchingWorkerCount,omitempty"`
	WatchingResyncCron  string            `json:"WatchingResyncCron,omitempty"`
	WatchingDebounce    *WatchingDebounce `json:"WatchingDebounce,omitempty"`

	// dubbo configs
	// configurator configs
	EnableConfiguratorMeta bool `json:"EnableConfiguratorMeta,omitempty"`

	// whether to gen dubbo `Sidecar`
	EnableDubboSidecar bool `json:"EnableDubboSidecar,omitempty"`
	// the removed dep service of an app will only be effective when so much time has passed (since last)
	TrimDubboRemoveDepInterval util.Duration `json:"TrimDubboRemoveDepInterval,omitempty"`
	// specify how to map `app` to label key:value pair
	DubboWorkloadAppLabel string `json:"DubboWorkloadAppLabel,omitempty"`
	// if true, will consider self-provided services as consumed services and add them to `Sidecar`
	SelfConsume bool `json:"SelfConsume,omitempty"`

	// if true, will consider a svc(IGV) will only be provided by one app. Thus, we can derive the `service-app`
	// from endpoints meta and set it to svc's label.
	SingleAppService bool `json:"SingleAppService,omitempty"`

	// specify which services will enable feature method-lb. supports dynamic config reload.
	// NOTICE:
	//   null or empty slice means "an empty whitelist list" and thus ALL-DISABLED;
	//   whitelist(selector with invert value false) has higher priority than blacklist, if there're any whitelists
	// all blacklists will be ignored.
	MethodLBServiceSelectors []*ServiceSelector `json:"MethodLBServiceSelectors,omitempty"`

	HostSuffix string `json:"HostSuffix,omitempty"`
	// default /consumers, set to `-` to empty it and disable fetching consumer data
	ConsumerPath string `json:"ConsumerPath,omitempty"`
}

type WatchingDebounce struct {
	TriggerCount     int           `json:"TriggerCount,omitempty"`
	DebounceDuration util.Duration `json:"DebounceDuration,omitempty"`
	Delay            util.Duration `json:"Delay,omitempty"`
	MaxDelay         util.Duration `json:"MaxDelay,omitempty"`
}

func (args *ZookeeperSourceArgs) Validate() error {
	if args == nil || !args.Enabled {
		return nil
	}

	if err := args.SourceArgs.Validate(); err != nil {
		return fmt.Errorf("invalid args for zookeeper source: %v", err)
	}

	if len(args.Address) == 0 {
		return errors.New("zookeeper server address must be set when zookeeper source is enabled")
	}
	return nil
}

func (args *ZookeeperSourceArgs) Rectify() {
	if args == nil {
		return
	}

	if args.GatewayModel {
		args.SvcPort = 80 // backwards compatible
		args.InstancePortAsSvcPort = false
		args.HostSuffix = ".dubbo"
		// gateway mode no need to get consumers information to generate sidecar
		args.EnableDubboSidecar = false
	} else if args.ConsumerPath == "" {
		args.ConsumerPath = defaultConsumerPath
	}

	if args.ConsumerPath == disableConsumerPath {
		args.ConsumerPath = ""
	}
}

type EurekaSourceArgs struct {
	SourceArgs
	EurekaServer
	// Deprecated
	// almost equals `EnableProjectCode = true && AppSuffix = ".nsf"` and will override them if true
	NsfEureka bool `json:"NsfEureka,omitempty"`
	// need k8sDomainSuffix in Host
	K8sDomainSuffix bool `json:"K8SDomainSuffix,omitempty"`
	// need ns in Host
	NsHost bool `json:"NsHost,omitempty"`
	// if true, will split service to project-services by project code
	EnableProjectCode bool `json:"EnableProjectCode,omitempty"`
	// if not empty, will add this suffix to app name
	AppSuffix string `json:"AppSuffix,omitempty"`

	Servers []EurekaServer `json:"Servers,omitempty"`
}

type EurekaServer struct {
	// RegistryID is the unique identifier of the eureka server.
	// If set, the registry id will be used as an entry in the endpoint metadata.
	RegistryID string   `json:"RegistryID,omitempty"`
	Address    []string `json:"Address,omitempty"`
}

func (eurekaServer *EurekaServer) Validate() error {
	if len(eurekaServer.Address) == 0 {
		return errors.New("eureka server address must be set")
	}
	return nil
}

func (args *EurekaSourceArgs) Validate() error {
	if args == nil || !args.Enabled {
		return nil
	}

	if err := args.SourceArgs.Validate(); err != nil {
		return fmt.Errorf("invalid args for eureka source: %v", err)
	}

	if len(args.Servers) == 0 {
		return args.EurekaServer.Validate()
	}
	for _, server := range args.Servers {
		err := server.Validate()
		if err != nil {
			return err
		}
	}
	return nil
}

func (args *EurekaSourceArgs) Rectify() {
	if args == nil {
		return
	}
	if args.GatewayModel {
		args.InstancePortAsSvcPort = false
		args.K8sDomainSuffix = false
	}
	if args.NsfEureka {
		args.EnableProjectCode = true
		args.AppSuffix = "nsf"
	}
}

type NacosSourceArgs struct {
	SourceArgs
	NacosServer
	// Deprecated
	// almost equals `EnableProjectCode = true && AppSuffix = ".nsf"` and will override them if true
	NsfNacos bool `json:"NsfNacos,omitempty"`
	// nacos mode for get nacos info
	Mode string `json:"Mode,omitempty"`
	// nacos service name is like name.ns
	NameWithNs bool `json:"NameWithNs,omitempty"`
	// need k8sDomainSuffix in Host
	K8sDomainSuffix bool `json:"K8SDomainSuffix,omitempty"`
	// need ns in Host
	NsHost bool `json:"NsHost,omitempty"`
	// If set, namespace and group information will be injected into the ep's metadata using the set key.
	MetaKeyGroup     string `json:"MetaKeyGroup,omitempty"`
	MetaKeyNamespace string `json:"MetaKeyNamespace,omitempty"`
	// if true, will split service to project-services by project code
	EnableProjectCode bool `json:"EnableProjectCode,omitempty"`
	// if not empty, will add this suffix to dom
	DomSuffix string        `json:"DomSuffix,omitempty"`
	Servers   []NacosServer `json:"Servers,omitempty"`
}

type NacosServer struct {
	// RegistryID is the unique identifier of the nacos server.
	// If set, the registry id will be used as an entry in the endpoint metadata.
	// The key of the entry is defined by env var `REGISTRY_ID_META_KEY`
	RegistryID string `json:"RegistryID,omitempty"`
	// addresses of the nacos servers
	Address []string `json:"Address,omitempty"`
	// namespace value for nacos client
	Namespace string `json:"Namespace,omitempty"`
	// group value for nacos client
	Group string `json:"Group,omitempty"`
	// username and password for nacos auth
	Username string `json:"Username,omitempty"`
	Password string `json:"Password,omitempty"`
	// fetch services from all namespaces, only support Polling mode
	AllNamespaces bool `json:"AllNamespaces,omitempty"`
	// NamespaceGroups specific multi namespace and multi groups split by comma. like:
	// - ns1:g1,g2
	// - ns2:g1,g2,g3
	// - ns3
	NamespaceGroups []string `json:"NamespaceGroups,omitempty"`
	// NamespaceToGroups store the mapping between namespace and groups,
	// build from the value of `NamespaceGroups` or `Namespace` and `Group`.
	NamespaceToGroups map[string][]string `json:"-"`
}

func (nacosServer *NacosServer) Validate() error {
	if len(nacosServer.Address) == 0 {
		return errors.New("nacos server address must be set")
	}
	return nil
}

func (nacosServer *NacosServer) Rectify() {
	if len(nacosServer.NamespaceGroups) == 0 {
		nacosServer.NamespaceToGroups = map[string][]string{
			nacosServer.Namespace: {nacosServer.Group},
		}
		return
	}

	nacosServer.NamespaceToGroups = make(map[string][]string, len(nacosServer.NamespaceGroups))
	for _, ngs := range nacosServer.NamespaceGroups {
		namespace, groups := parseNamespaceGroups(ngs)
		nacosServer.NamespaceToGroups[namespace] = groups
	}
}

func parseNamespaceGroups(ngs string) (string, []string) {
	parts := strings.SplitN(ngs, ":", 2)
	if len(parts) == 1 {
		return ngs, []string{"DEFAULT_GROUP"}
	}
	return parts[0], strings.Split(parts[1], ",")
}

func (args *NacosSourceArgs) Validate() error {
	if args == nil || !args.Enabled {
		return nil
	}
	if err := args.SourceArgs.Validate(); err != nil {
		return fmt.Errorf("invalid args for nacos source: %v", err)
	}
	if len(args.Servers) == 0 {
		return args.NacosServer.Validate()
	}
	for _, server := range args.Servers {
		err := server.Validate()
		if err != nil {
			return err
		}
	}
	return nil
}

func (args *NacosSourceArgs) Rectify() {
	if args == nil || !args.Enabled {
		return
	}
	if args.GatewayModel {
		args.InstancePortAsSvcPort = false
		args.K8sDomainSuffix = false
	}
	if args.NsfNacos {
		args.EnableProjectCode = true
		args.DomSuffix = "nsf"
	}
	args.NacosServer.Rectify()
	for i := range args.Servers {
		args.Servers[i].Rectify()
	}
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
	ret := &RegistryArgs{
		Args: a,
		Mcp: &McpArgs{
			ServerUrl:           "xds://0.0.0.0:16010",
			EnableAnnoResVer:    true,
			EnableIncPush:       true,
			CleanZombieInterval: 0,
		},
		K8S: &K8SArgs{
			ClusterID:                  features.ClusterName,
			ClusterRegistriesNamespace: podNamespace,
		},
		RegistryStartDelay: util.Duration(5 * time.Second),

		K8SSource: &K8SSourceArgs{
			SourceArgs:        SourceArgs{},
			EnableConfigFile:  false,
			WatchedNamespaces: metav1.NamespaceAll,
		},
		ZookeeperSource: &ZookeeperSourceArgs{
			SourceArgs: SourceArgs{
				RefreshPeriod:         util.Duration(10 * time.Second),
				LabelPatch:            true,
				SvcProtocol:           "DUBBO",
				InstancePortAsSvcPort: true,
				ResourceNs:            "dubbo",
			},
			IgnoreLabel:                []string{"pid", "timestamp", "dubbo"},
			Mode:                       "polling",
			WatchingWorkerCount:        10,
			ConnectionTimeout:          util.Duration(30 * time.Second),
			RegistryRootNode:           "/dubbo",
			TrimDubboRemoveDepInterval: util.Duration(24 * time.Hour),
			EnableDubboSidecar:         true,
			DubboWorkloadAppLabel:      "app",
		},
		EurekaSource: &EurekaSourceArgs{
			SourceArgs: SourceArgs{
				RefreshPeriod:         util.Duration(30 * time.Second),
				LabelPatch:            true,
				SvcProtocol:           "HTTP",
				SvcPort:               80,
				InstancePortAsSvcPort: true,
				// should set it to "xx" explicitly to get the same behaviour as before("foo.eureka")
				DefaultServiceNs: "eureka",
				ResourceNs:       "eureka",
			},
			K8sDomainSuffix: true,
			NsHost:          true,
		},
		NacosSource: &NacosSourceArgs{
			SourceArgs: SourceArgs{
				RefreshPeriod:         util.Duration(30 * time.Second),
				LabelPatch:            true,
				SvcProtocol:           "HTTP",
				SvcPort:               80,
				InstancePortAsSvcPort: true,
				DefaultServiceNs:      "nacos",
				ResourceNs:            "nacos",
			},
			Mode:            "polling",
			K8sDomainSuffix: true,
			NsHost:          true,
		},
	}

	return ret
}
