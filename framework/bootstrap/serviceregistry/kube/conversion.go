package kube

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"slime.io/slime/framework/bootstrap/resource"
	"slime.io/slime/framework/bootstrap/serviceregistry/model"
	"slime.io/slime/framework/bootstrap/viewstore"
)

const (
	SMTP    = 25
	DNS     = 53
	MySQL   = 3306
	MongoDB = 27017
)

var (
	// Ports be skipped for protocol sniffing. Applications bound to these ports will be broken if
	// protocol sniffing is enabled.
	wellKnownPorts = map[int32]struct{}{
		SMTP:    {},
		DNS:     {},
		MySQL:   {},
		MongoDB: {},
	}
	grpcWeb    = string(model.GRPCWeb)
	grpcWebLen = len(grpcWeb)
)

func ConvertSvcAndEps(cfg resource.Config, vs viewstore.ViewerStore) (*model.Service, []*model.IstioEndpoint, error) {
	// prepare service
	svc := cfg.Spec.(*corev1.Service)

	// prepare endpoint
	var endpoint *resource.Config
	// If service event was pushed before endpoint event, then related endpoint in viewStore will be nil.
	// So retry many times here
	gotEndpoint := false
	for i := 0; i < 100; i++ {
		endpoint = vs.Get(resource.Endpoints, cfg.Name, cfg.Namespace)
		if endpoint != nil {
			gotEndpoint = true
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	if !gotEndpoint {
		return nil, nil, fmt.Errorf("failed to get related endpoint: %s/%s", cfg.Namespace, cfg.Name)
	}
	ep := endpoint.Spec.(*corev1.Endpoints)

	addr := model.UnspecifiedIP
	if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != corev1.ClusterIPNone {
		addr = svc.Spec.ClusterIP
	}

	var labelSelectors map[string]string
	if svc.Spec.ClusterIP != corev1.ClusterIPNone && svc.Spec.Type != corev1.ServiceTypeExternalName {
		labelSelectors = svc.Spec.Selector
	}

	ports := make([]*model.Port, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		ports = append(ports, convertPort(port))
	}

	// build hostname
	hostname := ServiceHostname(svc.Name, svc.Namespace, model.DefaultTrustDomain)

	// build istioEndpoints
	ieps, err := ConvertIstioEndpoints(ep, hostname, svc.Name, svc.Namespace, vs)
	if err != nil {
		return nil, nil, err
	}

	istioService := &model.Service{
		Hostname:  hostname,
		Ports:     ports,
		Addresses: []string{addr},
		Attributes: model.ServiceAttributes{
			ServiceRegistry: model.Kubernetes,
			Name:            svc.Name,
			Namespace:       svc.Namespace,
			Labels:          svc.Labels,
			Annotations:     svc.Annotations,
			LabelSelectors:  labelSelectors,
		},
		Endpoints: ieps,
	}

	return istioService, ieps, nil
}

// ConvertIstioEndpoints transforms endpoints config to a list of IstioEndpoints.
func ConvertIstioEndpoints(ep *corev1.Endpoints, host model.Name, svcName, svcNamespace string,
	vs viewstore.ViewerStore,
) ([]*model.IstioEndpoint, error) {
	out := make([]*model.IstioEndpoint, 0)

	// prepare pods
	pods, err := generatePods(vs)
	if err != nil {
		return nil, err
	}
	for _, ss := range ep.Subsets {
		for _, ea := range ss.Addresses {
			// If endpoint event was pushed before pod event, then related pod in viewStore will be nil.
			// So retry many times here
			gotPod := false
			for i := 0; i < 100; i++ {
				pod := getPodByIP(ea.IP, pods)
				if pod != nil {
					gotPod = true
					for _, port := range ss.Ports {
						istioEndpoint := buildIstioEndpoint(ea.IP, port.Port, port.Name, svcName, svcNamespace, pod, []model.Name{host})
						out = append(out, istioEndpoint)
					}
					break
				}
				time.Sleep(1 * time.Millisecond)
				pods, err = generatePods(vs)
				if err != nil {
					return nil, err
				}
			}
			if !gotPod {
				return nil, fmt.Errorf("failed to get related pod for endpoint [%s/%s]", ep.Namespace, ep.Name)
			}
		}
	}

	return out, nil
}

func generatePods(vs viewstore.ViewerStore) ([]*corev1.Pod, error) {
	podCfgs, err := vs.List(resource.Pod, "")
	if err != nil {
		return nil, fmt.Errorf("list pods from view store error: %v", err)
	}
	var pods []*corev1.Pod
	for _, c := range podCfgs {
		pod := c.Spec.(*corev1.Pod)
		pods = append(pods, pod)
	}
	return pods, nil
}

func getPodByIP(ip string, pods []*corev1.Pod) *corev1.Pod {
	for _, pod := range pods {
		if ip == pod.Status.PodIP {
			return pod
		}
	}
	return nil
}

func convertPort(port corev1.ServicePort) *model.Port {
	return &model.Port{
		Name:     port.Name,
		Port:     int(port.Port),
		Protocol: convertProtocol(port.Port, port.Name, port.Protocol),
	}
}

func convertProtocol(port int32, portName string, proto corev1.Protocol) model.Instance {
	if proto == corev1.ProtocolUDP {
		return model.UDP
	}

	// If application protocol is set, we will use that
	// If not, use the port name
	name := portName

	// Check if the port name prefix is "grpc-web". Need to do this before the general
	// prefix check below, since it contains a hyphen.
	if len(name) >= grpcWebLen && strings.EqualFold(name[:grpcWebLen], grpcWeb) {
		return model.GRPCWeb
	}

	// Parse the port name to find the prefix, if any.
	i := strings.IndexByte(name, '-')
	if i >= 0 {
		name = name[:i]
	}

	p := model.Parse(name)
	if p == model.Unsupported {
		// Make TCP as default protocol for well know ports if protocol is not specified.
		if _, has := wellKnownPorts[port]; has {
			return model.TCP
		}
	}
	return p
}

// ServiceHostname produces FQDN for a k8s service
func ServiceHostname(name, namespace, domainSuffix string) model.Name {
	return model.Name(name + "." + namespace + "." + "svc" + "." + domainSuffix) // Format: "%s.%s.svc.%s"
}
