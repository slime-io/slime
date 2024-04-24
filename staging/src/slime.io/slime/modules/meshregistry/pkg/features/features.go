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

	EndpointRelabelItems = func() map[string]string {
		endpointRelabelItems := env.RegisterStringVar(
			"ENDPOINT_RELABEL_ITEMS",
			"",
			"specifies the label keys to re-label to another",
		).Get()
		return parseEndpointRelabelItems(endpointRelabelItems)
	}()

	IstioRevision = env.RegisterStringVar(
		"MESH_REG_ISTIO_REVISION",
		"",
		"specify the (istio) revision of mesh-registry which will be used to fill istio rev label to generated resources",
	).Get()

	ClusterName = env.RegisterStringVar("CLUSTER_ID", "Kubernetes",
		"defines the cluster that this mesh-registry instance is belongs to").Get()

	RegistryIDMetaKey = env.RegisterStringVar(
		"REGISTRY_ID_META_KEY",
		"registry-id",
		"specify the key of registry id in endpoint's metadata",
	).Get()

	DynamicConfigMap = env.RegisterStringVar(
		"DYNAMIC_CONFIG_MAP",
		"",
		"specify the name of config map which contains the dynamic configuration",
	).Get()

	WatchingRegistrySource = env.RegisterStringVar(
		"WATCHING_REGISTRYSOURCE",
		"",
		"specify which RegistrySource cr to watch, empty means no watching",
	).Get()

	SkipValidateTagValue = env.RegisterBoolVar(
		"SKIP_VALIDATE_LABEL_VALUE",
		false,
		"skip the validation of tag value",
	).Get()

	SeLabelKeys = func() *seLabelKeysHolder {
		seLabelSelectorKeys := env.RegisterStringVar(
			"SE_LABEL_SELECTOR_KEYS",
			"app",
			"specify the service entry label keys to select from endpoints separated by ',', "+
				"the format is <key1>[:<mapKey1>[:append]][,<key2>[:<mapKey2>[:append]]...], "+
				"key is the label key in endpoint, mapKey is the label key in service entry,"+
				"append is optional and default to false, if true, append the value to the mapKey.",
		).Get()
		return parseSeLabelKeys(seLabelSelectorKeys)
	}()

	NacosClientHeaders = env.RegisterStringVar(
		"NACOS_CLIENT_HEADERS",
		"",
		"specify the additional headers to send to nacos server",
	).Get()
)

func parseEndpointRelabelItems(endpointRelabelItems string) map[string]string {
	if endpointRelabelItems == "" {
		return nil
	}
	items := map[string]string{}
	for _, part := range strings.Split(endpointRelabelItems, ",") {
		if part == "" {
			continue
		}
		parts := strings.Split(part, "=")
		if len(parts) < 2 {
			continue
		}
		items[parts[0]] = parts[1]
	}
	return items
}
