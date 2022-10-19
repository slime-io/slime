module slime.io/slime/modules/bundle_example

go 1.16

require (
	slime.io/slime/framework v0.0.0
	slime.io/slime/modules/lazyload v0.0.0
	slime.io/slime/modules/limiter v0.0.0
	slime.io/slime/modules/plugin v0.0.0
)

replace (
	istio.io/api => istio.io/api v0.0.0-20211206163441-1a632586cbd4
	istio.io/istio-mcp => github.com/slime-io/istio-mcp v0.0.0-20220711081451-575e86a9da6e

	slime.io/slime/framework => ../../framework
	slime.io/slime/modules/lazyload => ../../staging/src/slime.io/slime/modules/lazyload
	slime.io/slime/modules/limiter => ../../staging/src/slime.io/slime/modules/limiter
	slime.io/slime/modules/plugin => ../../staging/src/slime.io/slime/modules/plugin
)
