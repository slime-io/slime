module slime.io/slime/modules/bundle_example

go 1.16

require (
	slime.io/slime/modules/lazyload v0.0.0
	slime.io/slime/modules/limiter v0.0.0
	slime.io/slime/modules/plugin v0.0.0
	slime.io/slime/framework v0.0.0

	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.17.2
)

replace (
	k8s.io/api => k8s.io/api v0.17.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.2

	slime.io/slime/modules/lazyload => ../../../lazyload
	slime.io/slime/modules/limiter => ../../../limiter
	slime.io/slime/modules/plugin => ../../../plugin
	slime.io/slime/framework => ../../framework
)