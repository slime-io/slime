module slime.io/slime/framework

go 1.16

require (
	github.com/envoyproxy/go-control-plane v0.9.9-0.20210217033140-668b12f5399d
	github.com/ghodss/yaml v1.0.0
	github.com/gogo/protobuf v1.3.2
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.5.2
	github.com/google/uuid v1.1.2
	github.com/hashicorp/go-multierror v1.1.1
	github.com/mitchellh/copystructure v1.2.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/orcaman/concurrent-map v0.0.0-20210106121528-16402b402231
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.26.0
	github.com/sirupsen/logrus v1.8.1
	go.uber.org/zap v1.19.0
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/yaml.v2 v2.4.0
	istio.io/api v0.0.0-20210322145030-ec7ef4cd6eaf
	istio.io/istio-mcp v0.0.0
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e
	k8s.io/kubectl v0.22.2
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	sigs.k8s.io/controller-runtime v0.10.3
)

replace (
	istio.io/api => istio.io/api v0.0.0-20211206163441-1a632586cbd4
	istio.io/istio-mcp => github.com/slime-io/istio-mcp v0.0.0-20230425025011-b36fb1902b29
)
