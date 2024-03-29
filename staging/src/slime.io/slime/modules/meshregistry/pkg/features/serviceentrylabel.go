package features

import (
	"strings"

	networkingapi "istio.io/api/networking/v1alpha3"
)

type seLabelKeyItem struct {
	key, mapKey string
	append      bool
}

type seLabelKeysHolder struct {
	// example: app:ourApp
	instanceLabels []*seLabelKeyItem
	// $-prefixed meta
	// support:
	//   $host : the first host of se. example: $host:theFirstHost
	seMeta []*seLabelKeyItem
}

func (h *seLabelKeysHolder) GenerateSeLabels(se *networkingapi.ServiceEntry) map[string]string {
	var labels map[string]string = map[string]string{}
	for _, item := range h.instanceLabels {
		if _, ok := labels[item.mapKey]; ok {
			continue
		}

		for _, ep := range se.Endpoints {
			if v, exist := ep.Labels[item.key]; exist {
				if item.append {
					labels[item.mapKey] = labels[item.mapKey] + "," + v
				} else {
					labels[item.mapKey] = v
				}
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
	return labels
}

func parseSeLabelKeys(s string) *seLabelKeysHolder {
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
		parts := strings.SplitN(part, ":", 3)
		item.key = parts[0]
		if len(parts) < 2 || parts[1] == "" {
			item.mapKey = parts[0]
		} else {
			item.mapKey = parts[1]
		}
		if len(parts) > 2 {
			item.append = parts[2] == "append"
		}
	}

	if len(instLabels) > 0 || len(seMeta) > 0 {
		return &seLabelKeysHolder{
			instanceLabels: instLabels,
			seMeta:         seMeta,
		}
	}
	return nil
}
