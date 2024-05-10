package util

import (
	"fmt"
	"regexp"

	networkingapi "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/resource"

	"slime.io/slime/modules/meshregistry/pkg/features"
	"slime.io/slime/modules/meshregistry/pkg/util/cache"
)

const (
	DNS1123LabelMaxLength = 63 // Public for testing only.
	dns1123LabelFmt       = "[a-zA-Z0-9](?:[-a-z-A-Z0-9]*[a-zA-Z0-9])?"

	// Using kubernetes requirement, a valid key must be a non-empty string consist
	// of alphanumeric characters, '-', '_' or '.', and must start and end with an
	// alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345'
	qualifiedNameFmt = "(?:[A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]"

	// In Kubernetes, label names can start with a DNS name followed by a '/':
	dnsNamePrefixFmt       = dns1123LabelFmt + `(?:\.` + dns1123LabelFmt + `)*/`
	dnsNamePrefixMaxLength = 253
)

var (
	// label value can be an empty string
	// TODO: replace by framework's definition
	tagRegexp        = regexp.MustCompile("^(" + dnsNamePrefixFmt + ")?(" + qualifiedNameFmt + ")$")
	labelValueRegexp = regexp.MustCompile("^" + "(" + qualifiedNameFmt + ")?" + "$")
)

func ValidateTagKey(k string) error {
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

func ValidateTagValue(v string) error {
	if features.SkipValidateTagValue {
		return nil
	}
	if !labelValueRegexp.MatchString(v) {
		return fmt.Errorf("invalid tag value: %q", v)
	}
	return nil
}

func FilterEndpointLabels(labels map[string]string, patchLabel bool, ip string, from string) {
	for k, v := range labels {
		if k == "@class" {
			delete(labels, k)
			continue
		}
		e := ValidateTagKey(k)
		if e != nil {
			log.Debugf("invalid label key: %s ;from %s", k, from)
			delete(labels, k)
			continue
		}
		e = ValidateTagValue(v)
		if e != nil {
			log.Debugf("invalid label value: %s ;from %s", v, from)
			delete(labels, k)
			continue
		}
	}

	for fromLabel, toLabel := range features.EndpointRelabelItems {
		if v, ok := labels[fromLabel]; ok {
			labels[toLabel] = v
		}
	}
	if patchLabel && ip != "" {
		meta, exist := cache.K8sPodCaches.Get(ip)
		if exist {
			for k, v := range meta.Labels {
				labels[k] = v
			}
		}
	}

	// fill istio-locality label
	locality := getLocality(labels, getNodeLabelsByPod(ip)) // final calculated locality
	if localityLabel := locality.LabelString(); localityLabel != "" {
		// XXX optimize avoid v -> label -> v
		labels[LocalityLabel] = localityLabel // use the istio-spec label to carry the final locality info
	}
}

func getNodeLabelsByPod(podIP string) map[string]string {
	if podIP == "" {
		return nil
	}

	nodeName, exist := cache.K8sPodCaches.GetHostKey(podIP)
	if !exist {
		return nil
	}
	meta, exist := cache.K8sNodeCaches.Get(nodeName)
	if !exist {
		return nil
	}
	labels := make(map[string]string, len(meta.Labels))
	for k, v := range meta.Labels {
		labels[k] = v
	}
	return labels
}

func CopySe(item *networkingapi.ServiceEntry) *networkingapi.ServiceEntry {
	newHosts := make([]string, len(item.Hosts))
	copy(newHosts, item.Hosts)
	newAddress := make([]string, len(item.Addresses))
	copy(newAddress, item.Addresses)
	newPorts := make([]*networkingapi.ServicePort, len(item.Ports))
	copy(newPorts, item.Ports)
	eps := make([]*networkingapi.WorkloadEntry, len(item.Endpoints))
	copy(eps, item.Endpoints) // XXX deep copy?
	newSe := &networkingapi.ServiceEntry{
		Hosts:           newHosts,
		Addresses:       newAddress,
		Ports:           newPorts,
		Location:        item.Location,
		Resolution:      item.Resolution,
		Endpoints:       eps,
		ExportTo:        item.ExportTo,
		SubjectAltNames: item.SubjectAltNames,
	}
	return newSe
}

func FillSeLabels(se *networkingapi.ServiceEntry, meta *resource.Metadata) bool {
	if features.SeLabelKeys == nil {
		return false
	}
	var changed bool
	labels := features.SeLabelKeys.GenerateSeLabels(se)
	for k, v := range labels {
		if exist, ok := meta.Labels[k]; !ok || exist != v {
			changed = true
			meta.Labels[k] = v
		}
	}

	return changed
}
