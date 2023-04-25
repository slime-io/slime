module slime.io/slime/modules/plugin

go 1.13

require (
	github.com/envoyproxy/go-control-plane v0.9.9-0.20210217033140-668b12f5399d
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.2
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/protobuf v1.28.1
	istio.io/api v0.0.0-20210322145030-ec7ef4cd6eaf
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	sigs.k8s.io/controller-runtime v0.10.3
	slime.io/slime/framework v0.0.0
)

replace (
	github.com/envoyproxy/go-control-plane => github.com/envoyproxy/go-control-plane v0.9.10-0.20210907150352-cf90f659a021
	istio.io/api => istio.io/api v0.0.0-20211206163441-1a632586cbd4
	istio.io/istio-mcp => github.com/slime-io/istio-mcp v0.0.0-20230425025011-b36fb1902b29

	slime.io/slime/framework => ../../../../../../framework
)
