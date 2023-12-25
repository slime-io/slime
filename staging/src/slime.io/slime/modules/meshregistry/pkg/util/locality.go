package util

import (
	"strings"

	networkingapi "istio.io/api/networking/v1alpha3"

	"slime.io/slime/modules/meshregistry/pkg/features"
)

const (
	// LocalityLabel indicates the region/zone/subzone of an instance. It is used to override the native
	// registry's value.
	//
	// Note: because k8s labels does not support `/`, so we use `.` instead in k8s.
	LocalityLabel = "istio-locality"

	// k8s istio-locality label separator
	k8sSeparator = "."

	IstioLocalitySeparator = "/"

	// copy from k8s api

	LabelTopologyZone   = "topology.kubernetes.io/zone"
	LabelTopologyRegion = "topology.kubernetes.io/region"
	// These label have been deprecated since 1.17, but will be supported for
	// the foreseeable future, to accommodate things like long-lived PVs that
	// use them.  New users should prefer the "topology.kubernetes.io/*"
	// equivalents.
	LabelFailureDomainBetaZone   = "failure-domain.beta.kubernetes.io/zone"   // deprecated
	LabelFailureDomainBetaRegion = "failure-domain.beta.kubernetes.io/region" // deprecated

	// NodeRegionLabel is the well-known label for kubernetes node region in beta
	NodeRegionLabel = LabelFailureDomainBetaRegion
	// NodeZoneLabel is the well-known label for kubernetes node zone in beta
	NodeZoneLabel = LabelFailureDomainBetaZone
	// NodeRegionLabelGA is the well-known label for kubernetes node region in ga
	NodeRegionLabelGA = LabelTopologyRegion
	// NodeZoneLabelGA is the well-known label for kubernetes node zone in ga
	NodeZoneLabelGA      = LabelTopologyZone
	TopologySubzoneLabel = "topology.istio.io/subzone"
)

var (
	RegionLabels  = []string{NodeRegionLabel, NodeRegionLabelGA}
	ZoneLabels    = []string{NodeZoneLabel, NodeZoneLabelGA}
	SubzoneLabels = []string{TopologySubzoneLabel}
)

func init() {
	if features.LocalityLabels != "" {
		var regionLabel, zoneLabel, subzoneLabel string
		parts := strings.SplitN(features.LocalityLabels, ",", 3)
		regionLabel = parts[0]
		if len(parts) > 1 {
			zoneLabel = parts[1]
		}
		if len(parts) > 2 {
			subzoneLabel = parts[2]
		}

		if regionLabel != "" {
			RegionLabels = append([]string{regionLabel}, RegionLabels...)
		}
		if zoneLabel != "" {
			ZoneLabels = append([]string{zoneLabel}, ZoneLabels...)
		}
		if subzoneLabel != "" {
			SubzoneLabels = append([]string{subzoneLabel}, SubzoneLabels...)
		}
	}
}

type IstioLocality struct {
	Region, Zone, Subzone string
}

var emptyIstioLocality = IstioLocality{}

func (l IstioLocality) String() string {
	return l.string(IstioLocalitySeparator)
}

func (l IstioLocality) string(sep string) string {
	if l.Region == "" {
		return "" // not allowed
	}
	if l == emptyIstioLocality {
		return ""
	}
	localitys := []string{}
	if len(l.Region) > 0 {
		localitys = append(localitys, l.Region)
	}
	if len(l.Zone) > 0 {
		localitys = append(localitys, l.Zone)
	}
	if len(l.Subzone) > 0 {
		localitys = append(localitys, l.Subzone)
	}
	// Format: "%s<sep>%s<sep>%s"
	return strings.Join(localitys, sep)
}

func (l IstioLocality) LabelString() string {
	return l.string(k8sSeparator)
}

func ParseIstioLocality(s string) IstioLocality {
	var (
		region, zone, subzone string
		parts                 = strings.SplitN(s, IstioLocalitySeparator, 3)
	)

	region = parts[0]
	if region == "" { // not allowed
		return emptyIstioLocality
	}
	if len(parts) > 1 {
		zone = parts[1]
	}
	if len(parts) > 2 {
		subzone = parts[2]
	}

	return IstioLocality{
		Region:  region,
		Zone:    zone,
		Subzone: subzone,
	}
}

// GetLocalityLabelOrDefault returns the locality from the supplied label, or falls back to
// the supplied default locality if the supplied label is empty. Because Kubernetes
// labels don't support `/`, we replace "." with "/" in the supplied label as a workaround.
func GetLocalityLabelOrDefault(label, defaultLabel string) string {
	if len(label) > 0 {
		// if there are /'s present we don't need to replace
		if strings.Contains(label, "/") {
			return label
		}
		// replace "." with "/"
		return strings.Replace(label, k8sSeparator, "/", -1)
	}
	return defaultLabel
}

func ConvertLabelValueToLocality(v string) string {
	return ParseIstioLocality(GetLocalityLabelOrDefault(v, "")).String()
}

// getLocality logic is copied from istio
func getLocality(lbl map[string]string, nodeLbl map[string]string) IstioLocality {
	// if has `istio-locality` label, skip below ops
	if locLabel := lbl[LocalityLabel]; locLabel != "" {
		return ParseIstioLocality(GetLocalityLabelOrDefault(locLabel, ""))
	}

	var region, zone, subzone string

	region = getLabelValue(lbl, RegionLabels...)
	if region == "" {
		region = getLabelValue(nodeLbl, RegionLabels...)
	}
	zone = getLabelValue(lbl, ZoneLabels...)
	if zone == "" {
		zone = getLabelValue(nodeLbl, ZoneLabels...)
	}
	subzone = getLabelValue(lbl, SubzoneLabels...)
	if subzone == "" {
		subzone = getLabelValue(nodeLbl, SubzoneLabels...)
	}

	return IstioLocality{Region: region, Zone: zone, Subzone: subzone}
}

// getLabelValue is copied from istio
func getLabelValue(labels map[string]string, labelsToTry ...string) string {
	for _, lbl := range labelsToTry {
		val := labels[lbl]
		if val != "" {
			return val
		}
	}

	return ""
}

func FillWorkloadEntryLocality(we *networkingapi.WorkloadEntry) {
	if we.Locality != "" {
		return
	}
	if v := we.Labels[LocalityLabel]; v != "" {
		we.Locality = ConvertLabelValueToLocality(v)
	}
}
