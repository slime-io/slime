module slime.io/slime/modules/lazyload

go 1.16

require (
	github.com/envoyproxy/go-control-plane v0.9.9-0.20210217033140-668b12f5399d
	github.com/gogo/protobuf v1.3.2
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.5.2
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/prometheus/client_golang v1.11.0
	github.com/sirupsen/logrus v1.8.1
	golang.org/x/sys v0.0.0-20210817190340-bfb29a6856f2
	helm.sh/helm/v3 v3.6.2
	istio.io/api v0.0.0-20210322145030-ec7ef4cd6eaf
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e
	sigs.k8s.io/controller-runtime v0.10.3
	sigs.k8s.io/yaml v1.2.0
	slime.io/slime/framework v0.0.0-00010101000000-000000000000
)

replace (
	istio.io/api => istio.io/api v0.0.0-20200807181912-0e773b04cfc7
	istio.io/istio-mcp => github.com/slime-io/istio-mcp v0.0.0-20230425025011-b36fb1902b29

	slime.io/slime/framework => ../../../../../../framework
)
