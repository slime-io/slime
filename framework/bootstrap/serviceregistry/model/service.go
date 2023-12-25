package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/mitchellh/copystructure"

	"slime.io/slime/framework/bootstrap/resource"
)

type Service struct {
	// Name of the istio service, e.g. "catalog.mystore.com"
	Hostname Name `json:"hostname"`

	// Attributes contains additional attributes associated with the service
	// used mostly by mixer and RBAC for policy enforcement purposes.
	Attributes ServiceAttributes

	// Ports is the set of network ports where the service is listening for
	// connections
	Ports PortList `json:"ports,omitempty"`

	// Address specifies the service IPv4 address of the load balancer
	// Do not access directly. Use GetServiceAddressForProxy
	Addresses []string `json:"addresses,omitempty"`

	// Related IstioEndpoint slice
	Endpoints []*IstioEndpoint
}

func (s *Service) DeepCopy() *Service {
	attrs := copyInternal(s.Attributes)
	ports := copyInternal(s.Ports)

	return &Service{
		Attributes: attrs.(ServiceAttributes),
		Ports:      ports.(PortList),
		Hostname:   s.Hostname,
		Addresses:  s.Addresses,
	}
}

func (s *Service) Reset() {
	*s = Service{}
}

func (s *Service) String() string {
	return fmt.Sprintf("{s.Hostname: %s, s.Attributes: %+v, s.Ports: %+v, s.Addresses: %+v, s.Endpoints: %+v}",
		s.Hostname, s.Attributes, s.Ports, s.Addresses, s.Endpoints)
}

func (s *Service) ConvertConfig() resource.Config {
	cfg := resource.Config{
		ConfigMeta: resource.ConfigMeta{
			GroupVersionKind:  resource.IstioService,
			Name:              string(s.Hostname),
			Namespace:         s.Attributes.Namespace,
			Domain:            "",
			Labels:            s.Attributes.Labels,
			Annotations:       s.Attributes.Annotations,
			ResourceVersion:   "",
			CreationTimestamp: time.Time{},
		},
		Spec: s,
	}
	return cfg
}

// ServiceAttributes represents a group of custom attributes of the service.
type ServiceAttributes struct {
	// ServiceRegistry indicates the backing service registry system where this service
	// was sourced from.
	// TODO: move the ServiceRegistry type from platform.go to model
	ServiceRegistry string
	// Name is "destination.service.name" attribute
	Name string
	// Namespace is "destination.service.namespace" attribute
	Namespace string
	// Labels is "destination.service.labels" attribute
	Labels map[string]string
	// Annotations is "destination.service.annotations" attribute
	Annotations map[string]string
	// LabelSelectors are the labels used by the service to select workloads.
	// Applicable to both Kubernetes and ServiceEntries.
	LabelSelectors map[string]string
}

func (s *ServiceAttributes) Reset() {
	*s = ServiceAttributes{}
}

func (s *ServiceAttributes) String() string {
	return fmt.Sprintf("{s.ServiceRegistry: %s, s.Name: %s, s.Namespace: %s, s.Labels: %+v, s.Annotations: %+v, s.LabelSelectors: %+v}",
		s.ServiceRegistry, s.Name, s.Namespace, s.Labels, s.Annotations, s.LabelSelectors)
}

type IstioEndpoint struct {
	// Address is the address of the endpoint, using envoy proto.
	Address string

	// Labels points to the workload or deployment labels.
	Labels LabelsInstance

	// Name of the istio service, e.g. "catalog.mystore.com"
	Hostnames []Name `json:"hostname"`

	// Name of the source service, e.g. k8s service or serviceEntry
	ServiceName string

	// Namespace of the source service
	Namespace string

	// ServicePortName tracks the name of the port, this is used to select the IstioEndpoint by service port.
	ServicePortName string

	// EndpointPort is the port where the workload is listening, can be different
	// from the service port.
	EndpointPort uint32
}

func (ep *IstioEndpoint) Reset() {
	*ep = IstioEndpoint{}
}

func (ep *IstioEndpoint) String() string {
	return fmt.Sprintf("{ep.Address: %s, ep.Labels: %+v, ep.Hostnames: %+v, ep.ServiceName: %s, ep.Namespace: %s, ep.ServicePortName: %s, ep.EndpointPort: %d}",
		ep.Address, ep.Labels, ep.Hostnames, ep.ServiceName, ep.Namespace, ep.ServicePortName, ep.EndpointPort)
}

func (ep *IstioEndpoint) DeepCopy() *IstioEndpoint {
	return copyInternal(ep).(*IstioEndpoint)
}

func (ep *IstioEndpoint) ConvertConfig() resource.Config {
	cfg := resource.Config{
		ConfigMeta: resource.ConfigMeta{
			GroupVersionKind:  resource.IstioEndpoint,
			Name:              ep.ServiceName + "/" + ep.ServicePortName + "/" + ep.Address + ":" + strconv.Itoa(int(ep.EndpointPort)),
			Namespace:         ep.Namespace,
			Domain:            "",
			Labels:            ep.Labels,
			Annotations:       nil, // workloadEntry does not contain annotations
			ResourceVersion:   "",
			CreationTimestamp: time.Now(),
		},
		Spec: ep,
	}
	return cfg
}

type Port struct {
	// Name ascribes a human readable name for the port object. When a
	// service has multiple ports, the name field is mandatory
	Name string `json:"name,omitempty"`

	// Port number where the service can be reached. Does not necessarily
	// map to the corresponding port numbers for the instances behind the
	// service.
	Port int `json:"port"`

	// Protocol to be used for the port.
	Protocol Instance `json:"protocol,omitempty"`
}

func (p *Port) Reset() {
	*p = Port{}
}

func (p *Port) String() string {
	return fmt.Sprintf("{p.Name: %s, p.Port: %d, p.Protocol: %s}", p.Name, p.Port, p.Protocol)
}

// PortList is a set of ports
type PortList []*Port

func copyInternal(v interface{}) interface{} {
	copied, err := copystructure.Copy(v)
	if err != nil {
		// There are 2 locations where errors are generated in copystructure.Copy:
		//  * The reflection walk over the structure fails, which should never happen
		//  * A configurable copy function returns an error. This is only used for copying times, which never returns an error.
		// Therefore, this should never happen
		panic(err)
	}
	return copied
}

// Instance defines network protocols for ports
type Instance string

const (
	// GRPC declares that the port carries gRPC traffic.
	GRPC Instance = "GRPC"
	// GRPCWeb declares that the port carries gRPC traffic.
	GRPCWeb Instance = "GRPC-Web"
	// HTTP declares that the port carries HTTP/1.1 traffic.
	// Note that HTTP/1.0 or earlier may not be supported by the proxy.
	HTTP Instance = "HTTP"
	// HTTP_PROXY declares that the port is a generic outbound proxy port.
	// Note that this is currently applicable only for defining sidecar egress listeners.
	// nolint
	HTTP_PROXY Instance = "HTTP_PROXY"
	// HTTP2 declares that the port carries HTTP/2 traffic.
	HTTP2 Instance = "HTTP2"
	// HTTPS declares that the port carries HTTPS traffic.
	HTTPS Instance = "HTTPS"
	// Thrift declares that the port carries Thrift traffic.
	Thrift Instance = "Thrift"
	// TCP declares the the port uses TCP.
	// This is the default protocol for a service port.
	TCP Instance = "TCP"
	// TLS declares that the port carries TLS traffic.
	// TLS traffic is assumed to contain SNI as part of the handshake.
	TLS Instance = "TLS"
	// UDP declares that the port uses UDP.
	// Note that UDP protocol is not currently supported by the proxy.
	UDP Instance = "UDP"
	// Mongo declares that the port carries MongoDB traffic.
	Mongo Instance = "Mongo"
	// Redis declares that the port carries Redis traffic.
	Redis Instance = "Redis"
	// MySQL declares that the port carries MySQL traffic.
	MySQL Instance = "MySQL"
	// Unsupported - value to signify that the protocol is unsupported.
	Unsupported Instance = "UnsupportedProtocol"
	// Dubbo qzmesh
	Dubbo Instance = "Dubbo"
)

// Parse from string ignoring case
func Parse(s string) Instance {
	switch strings.ToLower(s) {
	case "tcp":
		return TCP
	case "udp":
		return UDP
	case "grpc":
		return GRPC
	case "grpc-web":
		return GRPCWeb
	case "http":
		return HTTP
	case "http_proxy":
		return HTTP_PROXY
	case "http2":
		return HTTP2
	case "https":
		return HTTPS
	case "thrift":
		return Thrift
	case "tls":
		return TLS
	case "mongo":
		return Mongo
	case "redis":
		return Redis
	case "mysql":
		return MySQL
	case "dubbo":
		return Dubbo
	}

	return Unsupported
}

// Name describes a (possibly wildcarded) hostname
type Name string

const (
	DNS1123LabelMaxLength = 63 // Public for testing only.
	dns1123LabelFmt       = "[a-zA-Z0-9](?:[-a-zA-Z0-9]*[a-zA-Z0-9])?"
	// a wild-card prefix is an '*', a normal DNS1123 label with a leading '*' or '*-', or a normal DNS1123 label

	// Using kubernetes requirement, a valid key must be a non-empty string consist
	// of alphanumeric characters, '-', '_' or '.', and must start and end with an
	// alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345'
	qualifiedNameFmt = "(?:[A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]"

	// In Kubernetes, label names can start with a DNS name followed by a '/':
	dnsNamePrefixFmt       = dns1123LabelFmt + `(?:\.` + dns1123LabelFmt + `)*/`
	dnsNamePrefixMaxLength = 253
)

var tagRegexp = regexp.MustCompile("^(" + dnsNamePrefixFmt + ")?(" + qualifiedNameFmt + ")$") // label value can be an empty string

// LabelsInstance is a non empty map of arbitrary strings. Each version of a service can
// be differentiated by a unique set of labels associated with the version. These
// labels are assigned to all instances of a particular service version. For
// example, lets say catalog.mystore.com has 2 versions v1 and v2. v1 instances
// could have labels gitCommit=aeiou234, region=us-east, while v2 instances could
// have labels name=kittyCat,region=us-east.
type LabelsInstance map[string]string

// SubsetOf is true if the label has identical values for the keys
func (i LabelsInstance) SubsetOf(that LabelsInstance) bool {
	for k, v := range i {
		if that[k] != v {
			return false
		}
	}
	return true
}

// Equals returns true if the labels are identical
func (i LabelsInstance) Equals(that LabelsInstance) bool {
	if i == nil {
		return that == nil
	}
	if that == nil {
		return i == nil
	}
	return i.SubsetOf(that) && that.SubsetOf(i)
}

// Validate ensures tag is well-formed
func (i LabelsInstance) Validate() error {
	if i == nil {
		return nil
	}
	var errs error
	for k, v := range i {
		if err := validateTagKey(k); err != nil {
			errs = multierror.Append(errs, err)
		}
		_ = v
		// Due to dubbo improper tag value.
		//if !labelValueRegexp.MatchString(v) {
		//	errs = multierror.Append(errs, fmt.Errorf("invalid tag value: %q", v))
		//}
	}
	return errs
}

// validateTagKey checks that a string is valid as a Kubernetes label name.
func validateTagKey(k string) error {
	match := tagRegexp.FindStringSubmatch(k)
	if match == nil {
		return fmt.Errorf("invalid tag key: %q", k)
	}

	if len(match[1]) > 0 {
		dnsPrefixLength := len(match[1]) - 1 // exclude the trailing / from the length
		if dnsPrefixLength > dnsNamePrefixMaxLength {
			return fmt.Errorf("invalid tag key: %q (DNS prefix is too long)", k)
		}
	}

	if len(match[2]) > DNS1123LabelMaxLength {
		return fmt.Errorf("invalid tag key: %q (name is too long)", k)
	}

	return nil
}

const (
	External           = "External"
	Kubernetes         = "Kubernetes"
	UnspecifiedIP      = "0.0.0.0"
	Scheme             = "spiffe"
	DefaultTrustDomain = "cluster.local"
	UnixAddressPrefix  = "unix://"
)
