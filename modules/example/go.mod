module slime.io/slime/modules/example

go 1.16

require (
	github.com/golang/protobuf v1.5.3
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/protobuf v1.31.0
	istio.io/client-go v1.19.1
	k8s.io/apimachinery v0.28.0
	k8s.io/client-go v0.28.0
	sigs.k8s.io/controller-runtime v0.13.0
	slime.io/slime/framework v0.0.0
)

replace (
	istio.io/api => istio.io/api v1.19.1
	istio.io/istio-mcp => github.com/slime-io/istio-mcp v0.0.0-20230425025011-b36fb1902b29

	// slime.io/slime/framework => ../../../../../../framework   // need uncomment
	slime.io/slime/framework => ../../framework // need delete
)
