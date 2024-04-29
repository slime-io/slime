package util

import (
	"os"
)

var (
	EnvoyHTTPRateLimit         string
	EnvoyHTTPRouter            string
	EnvoyHTTPConnectionManager string
	EnvoyHTTPCors              string

	EnvoyGenericProxyRouter = "envoy.filters.generic.router"
	EnvoyDubboRouter        = "envoy.filters.dubbo.router"

	EnvoyGenericProxyPrefix = "generic-"
	EnvoyDubboProxy   = "envoy.filters.network.dubbo_proxy"
)

func init() {
	if os.Getenv("ENVOY_FILTER_NAME_LEGACY") != "" {
		EnvoyHTTPRateLimit = EnvoyHTTPRateLimitV1
		EnvoyHTTPRouter = EnvoyRouteV1
		EnvoyHTTPConnectionManager = EnvoyHTTPConnectionManagerV1
		EnvoyHTTPCors = EnvoyCorsV1
	} else {
		EnvoyHTTPRateLimit = EnvoyHTTPRateLimitV2
		EnvoyHTTPRouter = EnvoyRouteV2
		EnvoyHTTPConnectionManager = EnvoyHTTPConnectionManagerV2
		EnvoyHTTPCors = EnvoyCorsV2
	}
}

// v1 plugin names
const (
	EnvoyRatelimitV1             = "envoy.ratelimit"
	EnvoyHTTPRateLimitV1         = "envoy.rate_limit"
	EnvoyRouteV1                 = "envoy.router"
	EnvoyHTTPConnectionManagerV1 = "envoy.http_connection_manager"
	EnvoyCorsV1                  = "envoy.cors"
)

// v2 plugin names
const (
	EnvoyHTTPRateLimitV2         = "envoy.filters.http.ratelimit"
	EnvoyRouteV2                 = "envoy.filters.http.router"
	EnvoyHTTPConnectionManagerV2 = "envoy.filters.network.http_connection_manager"
	EnvoyCorsV2                  = "envoy.filters.http.cors"
)

const (
	EnvoyFilterHTTPWasm = "envoy.filters.http.wasm"
	EnvoyWasmV8         = "envoy.wasm.runtime.v8"
	EnvoyLocalRateLimit = "envoy.filters.http.local_ratelimit"
	EnvoyRateLimit      = "envoy.filters.http.ratelimit"

	StructWasmConfig        = "config"
	StructWasmName          = "name"
	StructWasmConfiguration = "configuration"

	StructAnyTypeURL = "type_url"
	StructAnyAtType  = "@type"
	StructAnyValue   = "value"

	StructHttpFilterTypedConfig     = "typed_config"
	StructHttpFilterName            = "name"
	StructHttpFilterConfigDiscovery = "config_discovery"
	StructHttpFilterConfigSource    = "config_source"
	StructHttpFilterDefaultConfig   = "default_config"
	StructHttpFilterAds             = "ads"
	StructHttpFilterTypeURLs        = "type_urls"

	StructFilterTypedPerFilterConfig = "typedPerFilterConfig"
	StructFilterPerFilterConfig      = "perFilterConfig"
	StructFilterDisabled             = "disabled"

	StructEnvoyLocalRateLimitLimiter  = "http_local_rate_limiter"
	StructEnvoyLocalRateLimitEnabled  = "local_rate_limit_enabled"
	StructEnvoyLocalRateLimitEnforced = "local_rate_limit_enforced"

	TypeURLEnvoyFilterHTTPWasm     = "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"
	TypeURLEnvoyFilterHTTPRider    = "type.googleapis.com/proxy.filters.http.rider.v3alpha1.FilterConfig"
	TypeURLStringValue             = "type.googleapis.com/google.protobuf.StringValue"
	TypeURLUDPATypedStruct         = "type.googleapis.com/udpa.type.v1.TypedStruct"
	TypeURLEnvoyLocalRateLimit     = "type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"
	TypeURLGenericProxyRouteAction = "type.googleapis.com/envoy.extensions.filters.network.generic_proxy.action.v3.RouteAction"
	TypeURLFiterConfig             = "type.googleapis.com/envoy.config.route.v3.FilterConfig"
	TypeURLConfigEmpty             = "type.googleapis.com/google.protobuf.Empty"

	EnvoyFilterGlobalSidecar = "to_global_sidecar"

	WellknownWildcard  = "*"
	WellknownRootpath  = "/"
	WellknownBaseSet   = "_base"
	WellknownK8sSuffix = ".svc.cluster.local"

	GlobalSidecar = "global-sidecar"
	MetricPrefix  = "slime"
)
