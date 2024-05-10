package server

import (
	// register eureka source
	_ "slime.io/slime/modules/meshregistry/pkg/source/eureka"
	// register k8s fs source
	_ "slime.io/slime/modules/meshregistry/pkg/source/k8s/fs"
	// register nacos source
	_ "slime.io/slime/modules/meshregistry/pkg/source/nacos"
	// register zookeeper source
	_ "slime.io/slime/modules/meshregistry/pkg/source/zookeeper"
)
