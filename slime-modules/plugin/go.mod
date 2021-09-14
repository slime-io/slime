module slime.io/slime/slime-modules/plugin

go 1.13

require (
	github.com/envoyproxy/go-control-plane v0.9.9-0.20201210154907-fd9021fe5dad
	github.com/go-logr/logr v0.1.0
	github.com/gogo/protobuf v1.3.2
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	go.uber.org/zap v1.10.0
	istio.io/api v0.0.0-20210322145030-ec7ef4cd6eaf
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.17.2
	sigs.k8s.io/controller-runtime v0.5.0
	slime.io/slime/slime-framework v0.0.0-00010101000000-000000000000
)

replace (
	k8s.io/api => k8s.io/api v0.17.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.2
	slime.io/slime/slime-framework => ../../slime-framework
)
