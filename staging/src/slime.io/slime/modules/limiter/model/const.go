package model

const (
	ConfigMapName = "slime-rate-limit-config"

	ConfigMapNamespace = "istio-system"

	ConfigMapConfig = "config.yaml"

	GenericKey = "generic_key"

	HeaderValueMatch = "header_match"

	Domain = "slime"

	Inbound = "inbound"

	Outbound = "outbound"

	// AllowAllPort use the implicit semantic "empty means match-all"
	AllowAllPort = ""

	GlobalSmartLimiter = "global"

	RateLimitService = "outbound|18081||rate-limit.istio-system.svc.cluster.local"

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
)
