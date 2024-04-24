package k8s

import (
	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/config/schema/collections"
	"istio.io/libistio/pkg/config/schema/resource"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

func BuildKubeSourceSchemas(cols, excludedKinds []string) collection.Schemas {
	builder := collection.NewSchemasBuilder()

	colMap := make(map[string]struct{})
	for _, col := range cols {
		colMap[col] = struct{}{}
	}
	excludeKindMap := make(map[string]struct{})
	for _, col := range excludedKinds {
		excludeKindMap[col] = struct{}{}
	}
	schemaMap := make(map[resource.Schema]struct{})
	for col := range colMap {
		var schemas collection.Schemas
		switch col {
		case bootstrap.CollectionsAll:
			schemas = collections.All
		case bootstrap.CollectionsIstio:
			schemas = collections.PilotGatewayAPI()
		case bootstrap.CollectionsLegacyDefault:
			schemas = collections.LegacyDefault
		case bootstrap.CollectionsLegacyLocal:
			schemas = collections.LegacyLocalAnalysis
		}
		for _, s := range schemas.All() {
			if _, ok := excludeKindMap[s.Kind()]; ok {
				continue
			}
			schemaMap[s] = struct{}{}
		}
	}
	for s := range schemaMap {
		builder.MustAdd(s)
	}
	return builder.Build()
}
