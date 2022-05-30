module slime.io/slime/modules/example

go 1.13

require (
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.4.2
	github.com/sirupsen/logrus v1.4.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.17.2
	sigs.k8s.io/controller-runtime v0.5.0
	slime.io/slime/framework v0.0.0
)

replace (
	k8s.io/api => k8s.io/api v0.17.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.2

	slime.io/slime/framework => ../../framework
)
