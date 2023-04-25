module slime.io/slime/modules/example

go 1.16

require (
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.2
	github.com/sirupsen/logrus v1.8.1
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	sigs.k8s.io/controller-runtime v0.10.3
)

require slime.io/slime/framework v0.0.0

replace (
	istio.io/api => istio.io/api v0.0.0-20200807181912-0e773b04cfc7
	istio.io/istio-mcp => github.com/slime-io/istio-mcp v0.0.0-20230425025011-b36fb1902b29

	// slime.io/slime/framework => ../../../../../../framework   // need uncomment
	slime.io/slime/framework => ../../framework // need delete
)
