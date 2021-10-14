module slime.io/slime/modules/bundle_example

go 1.13

require (
	slime.io/slime/modules/lazyload v0.0.0
	slime.io/slime/modules/limiter v0.0.0
	slime.io/slime/modules/plugin v0.0.0
	slime.io/slime/slime-framework v0.0.0
)

replace (
	slime.io/slime/modules/lazyload => ../../../lazyload
	slime.io/slime/modules/limiter => ../../../limiter
	slime.io/slime/modules/plugin => ../../../plugin
	slime.io/slime/slime-framework => ../../slime-framework
)