package features

import (
	"strings"

	"istio.io/libistio/pkg/env"
)

var (
	LocalityLabels = env.RegisterStringVar(
		"LOCALITY_LABELS",
		"",
		"specify the label keys used to derive locality info from pod/node labels and will be prior to "+
			"native keys. Value is in format <regionLabel>,<zoneLabel>,<subzoneLabel> and empty parts will be ignored",
	).Get()

	endpointRelabelItems = env.RegisterStringVar(
		"ENDPOINT_RELABEL_ITEMS",
		"",
		"specifies the label keys to re-label to another",
	).Get()

	IstioRevision = env.RegisterStringVar(
		"MESH_REG_ISTIO_REVISION",
		"",
		"specify the (istio) revision of mesh-registry which will be used to fill istio rev label to generated resources",
	).Get()

	ClusterName = env.RegisterStringVar("CLUSTER_ID", "Kubernetes",
		"defines the cluster that this mesh-registry instance is belongs to").Get()
)

var EndpointRelabelItems = map[string]string{}

func init() {
	if endpointRelabelItems != "" {
		for _, part := range strings.Split(endpointRelabelItems, ",") {
			if part == "" {
				continue
			}

			parts := strings.Split(part, "=")
			if len(parts) < 2 {
				continue
			}

			EndpointRelabelItems[parts[0]] = parts[1]
		}
	}
}
