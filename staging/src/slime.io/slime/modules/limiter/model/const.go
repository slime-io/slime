package model

const (
	ConfigMapName = "rate-limit-config"

	ConfigMapConfig = "config.yaml"

	GenericKey = "generic_key"

	HeaderValueMatch = "header_match"

	QueryMatch = "query_match"

	BodyMatch = "body_match"

	DescriptiorValue = "descriptor_value"

	Bodies = "bodies"

	RateLimits = "rate_limits"

	RateLimitActions = "actions"

	Route = "route"

	Domain = "slime"

	Inbound = "inbound"

	Outbound = "outbound"

	Gateway = "gateway"

	// AllowAllPort use the implicit semantic "empty means match-all"
	AllowAllPort = ""

	GlobalSmartLimiter = "global"

	SingleSmartLimiter = "single"

	AverageSmartLimiter = "average"

	TypeUrlEnvoyRateLimit = "type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit"

	StructDomain = "domain"

	StructRateLimitService = "rate_limit_service"

	TypePerFilterConfig = "typed_per_filter_config"

	EnvoyFiltersHttpRateLimit = "envoy.filters.http.ratelimit"

	EnvoyStatPrefix = "stat_prefix"

	EnvoyHttpLocalRateLimiterStatPrefix = "http_local_rate_limiter"

	MetricSourceTypePrometheus = "prometheus"

	MetricSourceTypeLocal = "local"

	MetricSourceType = "metric_source_type"

	InlineMetricPod = "pod"

	InboundDefaultRoute = "default"

	MockHost = "mock_host"
)
