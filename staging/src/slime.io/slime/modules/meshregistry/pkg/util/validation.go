package util

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	networkingapi "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/resource"

	"slime.io/slime/modules/meshregistry/pkg/features"
	"slime.io/slime/modules/meshregistry/pkg/util/cache"
)

const (
	DNS1123LabelMaxLength = 63 // Public for testing only.
	dns1123LabelFmt       = "[a-zA-Z0-9](?:[-a-z-A-Z0-9]*[a-zA-Z0-9])?"
	// a wild-card prefix is an '*', a normal DNS1123 label with a leading '*' or '*-', or a normal DNS1123 label
	wildcardPrefix = `(\*|(\*|\*-)?` + dns1123LabelFmt + `)`

	// Using kubernetes requirement, a valid key must be a non-empty string consist
	// of alphanumeric characters, '-', '_' or '.', and must start and end with an
	// alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345'
	qualifiedNameFmt = "(?:[A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]"

	// In Kubernetes, label names can start with a DNS name followed by a '/':
	dnsNamePrefixFmt       = dns1123LabelFmt + `(?:\.` + dns1123LabelFmt + `)*/`
	dnsNamePrefixMaxLength = 253
)

var (
	tagRegexp            = regexp.MustCompile("^(" + dnsNamePrefixFmt + ")?(" + qualifiedNameFmt + ")$") // label value can be an empty string
	labelValueRegexp     = regexp.MustCompile("^" + "(" + qualifiedNameFmt + ")?" + "$")
	dns1123LabelRegexp   = regexp.MustCompile("^" + dns1123LabelFmt + "$")
	wildcardPrefixRegexp = regexp.MustCompile("^" + wildcardPrefix + "$")
	seLabelKeys          *seLabelKeysHolder
	seLabelKeysStr       = "app"

	skipValidateTagValue = func() bool {
		switch os.Getenv("SKIP_VALIDATE_LABEL_VALUE") {
		case "1", "t", "T", "true", "TRUE", "True":
			return true
		}
		return false
	}()
)

type seLabelKeyItem struct {
	key, mapKey string
}

type seLabelKeysHolder struct {
	// example: app:ourApp
	instanceLabels []*seLabelKeyItem
	// $-prefixed meta
	// support:
	//   $host : the first host of se. example: $host:theFirstHost
	seMeta []*seLabelKeyItem
}

func (h *seLabelKeysHolder) selectLabelsInto(se *networkingapi.ServiceEntry, labels map[string]string) {
	for _, item := range h.instanceLabels {
		if _, ok := labels[item.mapKey]; ok {
			continue
		}

		for _, ep := range se.Endpoints {
			if v, exist := ep.Labels[item.key]; exist {
				labels[item.mapKey] = v
				break
			}
		}
	}

	for _, item := range h.seMeta {
		if _, ok := labels[item.mapKey]; ok {
			continue
		}

		switch item.key {
		case "host":
			var v string
			if len(se.Hosts) > 0 {
				v = se.Hosts[0]
			}
			labels[item.mapKey] = v
		}
	}
}

func parseSeLabelKeys(s string) (*seLabelKeysHolder, error) {
	var (
		instLabels []*seLabelKeyItem
		seMeta     []*seLabelKeyItem
	)

	for _, part := range strings.Split(s, ",") {
		item := &seLabelKeyItem{}

		if strings.HasPrefix(part, "$") {
			part = part[1:]
			seMeta = append(seMeta, item)
		} else {
			instLabels = append(instLabels, item)
		}

		if idx := strings.Index(part, ":"); idx >= 0 {
			item.key = part[:idx]
			item.mapKey = part[idx+1:]
		} else {
			item.key = part
			item.mapKey = part
		}
	}

	if len(instLabels) > 0 || len(seMeta) > 0 {
		return &seLabelKeysHolder{
			instanceLabels: instLabels,
			seMeta:         seMeta,
		}, nil
	}

	return nil, nil
}

func init() {
	// TODO move to source args
	if v := os.Getenv("SE_LABEL_SELECTOR_KEYS"); v != "" {
		// XXX can not override to "" ?
		seLabelKeysStr = v
	}
	if seLabelKeysStr != "" {
		if v, err := parseSeLabelKeys(seLabelKeysStr); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "invalid seLabelKeysStr %s ignored", seLabelKeysStr)
		} else {
			seLabelKeys = v
		}
	}
}

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
	if skipValidateTagValue {
		return nil
	}
	if !labelValueRegexp.MatchString(v) {
		return fmt.Errorf("invalid tag value: %q", v)
	}
	return nil
}

func FilterLabels(labels map[string]string, patchLabel bool, ip string, from string) {
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

func SelectLabels(item *networkingapi.ServiceEntry) map[string]string {
	labels := make(map[string]string, 0)

	if seLabelKeys != nil {
		seLabelKeys.selectLabelsInto(item, labels)
	}
	return labels
}

func FillSeLabels(se *networkingapi.ServiceEntry, meta resource.Metadata) bool {
	var (
		labels  = SelectLabels(se)
		changed bool
	)

	for k, v := range labels {
		if exist, ok := meta.Labels[k]; !ok || exist != v {
			changed = true
			meta.Labels[k] = v
		}
	}

	return changed
}
