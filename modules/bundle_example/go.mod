module slime.io/slime/modules/bundle_example

go 1.16

require (
	slime.io/slime/framework v0.0.0
	slime.io/slime/modules/lazyload v0.0.0
	slime.io/slime/modules/limiter v0.0.0
	slime.io/slime/modules/meshregistry v0.0.0-00010101000000-000000000000
	slime.io/slime/modules/plugin v0.0.0
)

replace (
	github.com/go-zookeeper/zk => github.com/slime-io/go-zk v0.0.0-20220815024955-60853abc02f4
	github.com/prometheus/common => github.com/prometheus/common v0.26.0

	istio.io/api => istio.io/api v0.0.0-20211206163441-1a632586cbd4
	istio.io/istio-mcp => github.com/slime-io/istio-mcp v0.0.0-20221025054010-d76433980485

	istio.io/libistio => github.com/slime-io/libistio v0.0.0-20221214030325-22f5add50855

	slime.io/slime/framework => ../../framework
	slime.io/slime/modules/lazyload => ../../staging/src/slime.io/slime/modules/lazyload
	slime.io/slime/modules/limiter => ../../staging/src/slime.io/slime/modules/limiter
	slime.io/slime/modules/meshregistry => ../../staging/src/slime.io/slime/modules/meshregistry
	slime.io/slime/modules/plugin => ../../staging/src/slime.io/slime/modules/plugin
)
