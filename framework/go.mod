module slime.io/slime/framework

go 1.16

require (
	contrib.go.opencensus.io/exporter/prometheus v0.4.0
	github.com/envoyproxy/go-control-plane v0.11.0
	github.com/ghodss/yaml v1.0.0
	github.com/gogo/protobuf v1.3.2
	github.com/golang/glog v1.0.0
	github.com/golang/protobuf v1.5.3
	github.com/google/uuid v1.3.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/mitchellh/copystructure v1.2.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.24.1
	github.com/orcaman/concurrent-map v0.0.0-20210106121528-16402b402231
	github.com/prometheus/client_golang v1.16.0
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.42.0
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.8.2 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.24.0
	golang.org/x/oauth2 v0.8.0 // indirect
	google.golang.org/grpc v1.52.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/yaml.v2 v2.4.0
	istio.io/api v0.0.0-20210322145030-ec7ef4cd6eaf
	istio.io/istio-mcp v0.0.0
	istio.io/pkg v0.0.0-20200807181912-d97bc429be20
	k8s.io/api v0.26.0
	k8s.io/apimachinery v0.26.0
	k8s.io/client-go v0.26.0
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20221012153701-172d655c2280
	k8s.io/kubectl v0.22.2
	k8s.io/utils v0.0.0-20221128185143-99ec85e7a448
	sigs.k8s.io/controller-runtime v0.14.0
)

replace (
	istio.io/api => istio.io/api v0.0.0-20211206163441-1a632586cbd4
	istio.io/istio-mcp => github.com/slime-io/istio-mcp v0.0.0-20230425025011-b36fb1902b29
)
