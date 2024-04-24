package server

import (
	_ "slime.io/slime/modules/meshregistry/pkg/source/eureka"
	_ "slime.io/slime/modules/meshregistry/pkg/source/k8s/fs"
	_ "slime.io/slime/modules/meshregistry/pkg/source/nacos"
	_ "slime.io/slime/modules/meshregistry/pkg/source/zookeeper"
)
