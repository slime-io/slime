## 1.背景

为了让更多开发者快速融入slime社区，本文将提供一份开发指南，指引开发者在slime项目中创建子模块、功能开发、编译构建、部署，功能验证

## 2.创建模块

slime提供了一套脚手架, 通过该脚手架，使用者可以快速创建一个包含了reconcile功能的子模块，这样开发人员就可以将更多的精力放在应用程序的逻辑实现上，加速了开发流程。

### 2.1 创建子模块

运行以下命令，在slime的`staging/src/slime.io/slime/modules`目录下创建子模块foo，开发者可自行替换子模块名称

```shell
bash bin/gen_module.sh foo
```

新创建的子模块目录树如下：

```
|-- Dockerfile
|-- PROJECT
|-- README.md
|-- api
|   |-- config
|   |   |-- foo_module.pb.go
|   |   |-- foo_module.proto
|   |   `-- foo_module_deepcopy.gen.go
|   `-- v1alpha1
|       |-- foo.pb.go
|       |-- foo.proto
|       |-- foo_deepcopy.gen.go
|       |-- foo_types.go
|       |-- groupversion_info.go
|       `-- zz_generated.deepcopy.go
|-- charts
|   `-- values.yaml
|-- controllers
|   `-- foo_controller.go
|-- go.mod
|-- go.sum
|-- install
|   `-- samples
|       `-- slimeboot_foo.yaml
|-- main.go
|-- model
|   `-- model.go
|-- module
|   `-- module.go
`-- publish.sh
```

其中的一些目录含义如下：

- api：
    - config：子模块的启动参数定义，例如，limiter定义的disableGlobalRateLimit字段，在启动时根据该参数关闭全局共享限流功能
    - v1alpha1：子模块的CRD定义，默认情况下生成的CRD的gvk是`microservice.slime.io/v1alpha1/Foo`,其中group和version固定不变，kind等同于用户的子模块名称
- controller: 核心的controller逻辑，开发者需要关注的目录
- install: 根据该内容部署你的子模块
- publish: 镜像构建脚本

### 2.2 修改api

在2.1中，我们创建了一个子模块foo，其api定义位于`api/v1alpha1`，默认情况下其定义如下

```go
type Foo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FooSpec   `json:"spec,omitempty"`
	Status FooStatus `json:"status,omitempty"`
}

// FooSpec defines the desired state of Foo
type FooSpec struct {
	// Foo is an foo field of Foo.
	Foo string `protobuf:"bytes,1,opt,name=foo,proto3" json:"foo,omitempty"`
	// Foo2 is an foo field of Foo.
	Foo2 string `protobuf:"bytes,2,opt,name=foo2,proto3" json:"foo2,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}
```

如果使用者需要修改spec内容, 比如想给foo的spec加上foo3字段,你可以修改`api/v1alpha1/foo.proto`

样例如下：

```proto
message FooSpec {
  // Foo is an foo field of Foo.
  string foo = 1;

  // Foo2 is an foo field of Foo.
  string foo2 = 2;

  // Foo3 is an foo field of Foo.
  string foo3 = 3;
}
```

然后执行以下命令(需要docker)，更新api内容, 其中MODULES的值需要替换成你创建时定义的模块名称

**note: 每次修改proto后都需要执行该命令**
```shell
MODULES=foo make generate-module
```

之后观察spec是否更新

```go
type Foo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FooSpec   `json:"spec,omitempty"`
	Status FooStatus `json:"status,omitempty"`
}

type FooSpec struct {
	// Foo is an foo field of Foo.
	Foo string `protobuf:"bytes,1,opt,name=foo,proto3" json:"foo,omitempty"`
	// Foo2 is an foo field of Foo.
	Foo2 string `protobuf:"bytes,2,opt,name=foo2,proto3" json:"foo2,omitempty"`
	// Foo3 is an foo field of Foo.
	Foo3                 string   `protobuf:"bytes,3,opt,name=foo3,proto3" json:"foo3,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}
```
**note: proto中的oneof类型目前不能优雅支持，请使用者谨慎使用**

### 2.3 生成CRD

在执行2.2后的，在`charts/crds`目录下生成了`microservice.slime.io/v1alpha1/Foo`

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.7.0
  creationTimestamp: null
  name: foos.microservice.slime.io
spec:
  group: microservice.slime.io
  names:
    kind: Foo
    listKind: FooList
    plural: foos
    singular: foo
  scope: Namespaced
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: Foo is the Schema for the foos API
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: FooSpec defines the desired state of Foo
            properties:
              foo:
                description: Foo is an foo field of Foo.
                type: string
              foo2:
                description: Foo2 is an foo field of Foo.
                type: string
              foo3:
                description: Foo3 is an foo field of Foo.
                type: string
            type: object
          status:
            description: FooStatus defines the observed state of Foo
            properties:
              bar:
                description: Bar is an foo field of FooStatus.
                type: string
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
```

## 3.功能开发

子模块的核心处理逻辑在 `staging/src/slime.io/slime/modules/foo/controllers/foo_controller.go`

使用者可以编写自己的协调过程，一个简单的样例如下，用户可自行参考

```go

import(
     "slime.io/slime/modules/foo/api/v1alpha1"
      log "github.com/sirupsen/logrus"
)

// 创建/更新/删除时 打印相关日志 
func (r *FooReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Infof("begin reconcile, get foo %+v", req)
	instance := &foov1alpha1.Foo{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			instance = nil
			err = nil
			log.Infof("foo %v not found", req.NamespacedName)
		} else {
			log.Errorf("get foo %v err, %s", req.NamespacedName, err)
			return reconcile.Result{}, err
		}
	}
	// deleted
	if instance == nil {
		log.Infof("foo %v is deleted", req)
		return reconcile.Result{}, nil
	} else {
		// add or update
		log.Infof("foo %v is added or updated", req)
	}
	return reconcile.Result{}, nil
}
```

## 4.编译构建

- 首先切换至子模块目录 `slime/staging/src/slime.io/slime/modules/foo`

- 之后执行以下命令编译构建镜像
```shell
./publish.sh build image`
```
也可以直接将编译构建镜像并推送至镜像仓库，**需要指定HUB**
```shell
HUB=xxx ./publish.sh build image image-push
```
**note**: slime模块的编译构建需要docker的buildx特性，一个简单的开启方式: 在 `~/.docker/config.json`增加 `"experimental": "enabled"`

更多的编译构建信息请参考 [build](./slime-build.md)

## 5.部署

在slime中我们提供了一种slimeboot的部署方式，关于slimeboot的更多信息，可以参考 [slime-boot](./slime-boot.md)

用户只需要提供 SlimeBoot CR（包含部署的模块信息），就可以部署你的子模块，前提是需要安装SlimeBoot相关CRD和Deployment

### 5.1 SlimeBoot安装

首先需要安装 [SlimeBoot-CRD](../../install/init/slimeboots-v1.yaml), [SlimeBoot-deployment](../../install/init/deployment_slime-boot.yaml)

### 5.2 创建Foo CRD

需要将microservice.slime.io_foos注册到k8s集群, 资源位于子模块`chart/crds/microservice.slime.io_foos.yaml`

### 5.3 部署应用

创建CRD后，使用者需要部署具体的应用，一般情况下使用的是chart, 但slime目前支持了SlimeBoot的部署方式

在新生成的子模块`install/samples/simeboot_foo.yaml`路径下，自动生成了的foo模块的SlimeBoot

使用者在apply该份yaml前，需要修改image信息，以及将name/kind修改成对应的模块名称

**如果没有 mesh-operator 可以先创建一个ns**

SlimeBoot的内容大致如下：

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: foo
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-foo
    tag: ${IMAGE}
  module:
    - name: foo
      kind: foo
      enable: true
      global:
        log:
          logLevel: info
```

### 5.4 pods 验证

经过以上三步，你会在你的mesh-operator发现新的deployment

- foo: 你创建的子模块
- slime-boot: slimeboot deployment

```
kubectl get deploy -n mesh-operator
NAME         READY   UP-TO-DATE   AVAILABLE   AGE
foo          1/1     1            1           81m
slime-boot   1/1     1            1           84m
```

### 5.4 功能验证

在之前的几个步骤中，我们注册了CRD，部署了服务，现在我们需要下发一份Foo CR验证服务是否正常。

**note: CR需要使用者根据自己的api自行修改**

apply以下CR之后再删除，观察你部署的pods日志

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: Foo
metadata:
  labels:
  name: test
  namespace: default
spec:
  foo: a
  foo2: b
  foo3: c 
```

如果你按照第3步修改，将会出现以下日志，也证明你新创建的子模块正常work

```
// 创建
time=2022-12-08T07:33:52Z level=info msg=begin reconcile, get foo default/test
time=2022-12-08T07:33:52Z level=info msg=foo default/test is added or updated

// 删除
time=2022-12-08T07:34:52Z level=info msg=begin reconcile, get foo default/test
time=2022-12-08T07:34:52Z level=info msg=foo default/test not found
time=2022-12-08T07:34:52Z level=info msg=foo default/test is deleted
```

到这里我们介绍了子模块创建、功能开发、编译构建、部署使用，功能验证。