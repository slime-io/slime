module slime.io/slime/modules/limiter

go 1.13

require (
	github.com/envoyproxy/go-control-plane v0.9.9
	github.com/gogo/protobuf v1.3.2
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.3
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/orcaman/concurrent-map v0.0.0-20210106121528-16402b402231
	github.com/prometheus/client_golang v1.0.0
	github.com/sirupsen/logrus v1.4.2
	gopkg.in/yaml.v2 v2.3.0
	istio.io/api v0.0.0-20210322145030-ec7ef4cd6eaf
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.17.2
	sigs.k8s.io/controller-runtime v0.5.0
	slime.io/slime/framework v0.0.0-00010101000000-000000000000
)

replace (
	istio.io/api => istio.io/api v0.0.0-20200807181912-0e773b04cfc7
	istio.io/istio-mcp => github.com/slime-io/istio-mcp v0.0.0-20220711081451-575e86a9da6e
	k8s.io/api => k8s.io/api v0.17.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.2

	slime.io/slime/framework => ../../../../../../framework
)
