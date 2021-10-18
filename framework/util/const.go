package util

const (
	// Plugins
	Envoy_Ratelimit             = "envoy.ratelimit"
	Envoy_Cors                  = "envoy.cors"
	Envoy_FilterHttpWasm        = "envoy.filters.http.wasm"
	Envoy_WasmV8                = "envoy.wasm.runtime.v8"
	Envoy_Route                 = "envoy.router"
	Envoy_HttpConnectionManager = "envoy.http_connection_manager"
	Envoy_LocalRateLimit        = "envoy.filters.http.local_ratelimit"
	Netease_LocalFlowControl    = "com.netease.local_flow_control"

	Struct_Wasm_Config        = "config"
	Struct_Wasm_Name          = "name"
	Struct_Wasm_Configuration = "configuration"

	Struct_Any_TypedUrl = "type_url"
	Struct_Any_AtType   = "@type"
	Struct_Any_Value    = "value"

	Struct_HttpFilter_TypedConfig          = "typed_config"
	Struct_HttpFilter_Name                 = "name"
	Struct_HttpFilter_TypedPerFilterConfig = "typedPerFilterConfig"

	Struct_EnvoyLocalRateLimit_Limiter  = "http_local_rate_limiter"
	Struct_EnvoyLocalRateLimit_Enabled  = "local_rate_limit_enabled"
	Struct_EnvoyLocalRateLimit_Enforced = "local_rate_limit_enforced"

	TypeUrl_EnvoyFilterHttpWasm     = "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"
	TypeUrl_StringValue             = "type.googleapis.com/google.protobuf.StringValue"
	TypeUrl_UdpaTypedStruct         = "type.googleapis.com/udpa.type.v1.TypedStruct"
	TypeUrl_EnvoyLocalRatelimit     = "type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"
	TypeUrl_NeteaseLocalFlowControl = "type.googleapis.com/netease.filters.http.local_flow_control.v2"

	EnvoyFilter_GlobalSidecar = "to_global_sidecar"

	Wellknow_Waidcard  = "*"
	Wellknow_RootPath  = "/"
	Wellkonw_BaseSet   = "_base"
	Wellkonw_K8sSuffix = ".svc.cluster.local"
)
