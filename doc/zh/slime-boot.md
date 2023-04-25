- [SlimeBoot 介绍与使用](#slimeboot-介绍与使用)
  - [介绍](#介绍)
  - [准备](#准备)
  - [参数介绍](#参数介绍)
  - [安装](#安装)
    - [lazyload安装样例](#lazyload安装样例)
    - [limiter安装样例](#limiter安装样例)
    - [plugin 安装样例](#plugin-安装样例)
    - [meshregistry安装样例](#meshregistry安装样例)
    - [bundle模式安装样例](#bundle模式安装样例)
    - [多副本样例](#多副本样例)
    - [Config.global](#configglobal)



# SlimeBoot 介绍与使用

## 介绍

本文将介绍`SlimeBoot`的使用方式，并给出使用样例，指引用户安装并使用`slime`组件。`slime-boot`可以理解成一个`Controller`，它会一直监听`SlimeBoot CR`，当用户提交一份`SlimeBoot CR`后，`slime-boot Controller`会根据`CR`的内容渲染`slime`相关的部署材料。

## 准备

在安装`slime`组件前，需要安装`SlimeBoot CRD`和`deployment/slime-boot`

**注意**：在k8s v1.22以及之后的版本中，只支持`apiextensions.k8s.io/v1`版本的`CRD`，不再支持`apiextensions.k8s.io/v1beta1`版本`CRD`，详见[k8s官方文档](https://kubernetes.io/docs/reference/using-api/deprecation-guide/#customresourcedefinition-v122)，而 k8s v1.16~v1.21 两个版本都能用

1. 对于k8s v1.22以及之后版本，需要手动安装[v1版本-crd](../../install/init/crds-v1.yaml)，而之前版本可手动安装[v1版本-crd](../../install/init/crds-v1.yaml)或者[v1beta1版本-crd](../../install/init/crds.yaml)
2. 手动安装 [deployment/slime-boot](../../install/init/deployment_slime-boot.yaml)

或者执行以下命令安装`slimeboot CRD`和`deployment/slime-boot`，需要注意的是如果网络无法访问，你可以在项目的`slime/install/init/`目录发现相关文档 

- k8s version >= v1.22
```shell
export tag_or_commit=$(curl -s https://api.github.com/repos/slime-io/slime/tags | grep 'name' | cut -d\" -f4 | head -1)
kubectl create ns mesh-operator
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/init/crds-v1.yaml"
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/init/deployment_slime-boot.yaml"
```

- k8s v1.16 <= version < 1.22
  以下两者都能使用
```shell
export tag_or_commit=$(curl -s https://api.github.com/repos/slime-io/slime/tags | grep 'name' | cut -d\" -f4 | head -1)
kubectl create ns mesh-operator
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/init/crds.yaml"
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/init/deployment_slime-boot.yaml"
```

```shell
export tag_or_commit=$(curl -s https://api.github.com/repos/slime-io/slime/tags | grep 'name' | cut -d\" -f4 | head -1)
kubectl create ns mesh-operator
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/init/crds-v1.yaml"
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/init/deployment_slime-boot.yaml"
```

## 参数介绍

根据之前的章节，我们知道用户通过下发`SlimeBoot`的方式，安装`slime`组件，在正常使用过程中，用户使用的`SlimeBoot CR`主要包含以下几项

- image: 定义镜像相关的参数
- resources： 定义容器资源
- module: 定义需要启动的模块，以及对应的参数
  - name: 模块名称
  - kind：模块类别，目前只支持 lazyload/plugin/limiter/meshregistry
  - enable: 是否开启模块
  - global: 模块依赖的一些全局参数，一些详细信息可以参考 [Config.global](#configglobal)
  - general: 模块启动时需要的一些参数

样例如下：

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: xxx ## real name
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-xxx ## real image
    tag: xxx  ## real image
  module:
    - name: xxx
      kind: xxx
      enable: true
      global:
        log:
          logLevel: info
```

## 安装

`SlimeBoot`支持两种安装方式

- 模块部署：需要给每个子模块部署一份`deployment`

以下yaml是一个模块部署不完整样例，module中定义的第一个对象，就是该`SlimeBoot`应该具备的功能

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: xxx ## real name
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-xxx ## real image
    tag: xxx  ## real image
  module:
    - name: xxx
      kind: xxx
      enable: true
      global:
        log:
          logLevel: info
```

- bundle部署：只需部署一份`deployment`，该`deployment`包含多个组件功能

以下yaml是一个bundle模式不完整样例，其中module的第一个对象定义了这个服务拥有limiter和plugin功能，mudule中后两个对象分别对应limiter和plugin子模块的具体参数

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: bundle
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-bundle-all
    tag: v0.7.1
  module:
    - name: bundle
      enable: true
      bundle:
        modules:
          - name: limiter
            kind: limiter
          - name: plugin
            kind: plugin        
    - name: limiter
      kind: limiter
      enable: true
      mode: BundleItem
      general: {}
      global: {}
    - name: plugin
      kind: plugin
      enable: true
      mode: BundleItem
```


下面将以模块方式安装`lazyload`，`limiter`和`plugin`模块以及用`bundle`模式安装`bundle`模块


###  lazyload安装样例

部署支持`cluster`级别的懒加载模块，成功后会在`mesh-operator`命名空间下部署名为`lazyload`和`global-sidecar`的`deployment`

- istioNamespace：用户集群中，`istio`部署的`ns`
- module: 指定`lazyload`部署的相关参数
  - name: 模块名称
  - kind：模块类别，目前只支持 lazyload/plugin/limiter/meshregistry
  - enable: 是否开启该模块
  - general: `lazyload`启动相关参数
  - global: `lazyload`依赖的一些全局参数，global具体参数可参考 [Config.global](#configglobal)
  - metric: `lazyload`服务间电泳关系依赖的指标信息
- component：懒加载模块中关于`globalSidecar`的配置，除镜像外一般不用改动

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-lazyload
    tag: v0.7.1
  namespace: mesh-operator
  istioNamespace: istio-system
  module:
    - name: lazyload
      kind: lazyload
      enable: true
      general:
        autoPort: true
        autoFence: true
        defaultFence: true   
        wormholePort: # replace to your application service ports, and extend the list in case of multi ports
          - "9080"
      global:
        log:
          logLevel: info
        misc:
          globalSidecarMode: cluster # the mode of global-sidecar
          metricSourceType: accesslog # indicate the metric source
        slimeNamespace: mesh-operator
  resources:
    requests:
      cpu: 300m
      memory: 300Mi
    limits:
      cpu: 600m
      memory: 600Mi        
  component:
    globalSidecar:
      enable: true
      sidecarInject:
        enable: true # should be true
        mode: pod
        labels: # optional, used for sidecarInject.mode = pod
          sidecar.istio.io/inject: "true"
      resources:
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 400m
          memory: 400Mi
      image:
        repository: docker.io/slimeio/slime-global-sidecar
        tag: v0.7.1
      probePort: 20000
```

### limiter安装样例

安装支持单机限流功能的限流模块，成功后会在`mesh-operator`命名空间下部署名为`limiter`的`deployment`

- image: 指定`limiter`的镜像，包括策略，仓库，tag
- module: 指定`limiter`部署的相关参数
  - name: 模块名称
  - kind：模块类别，目前只支持 lazyload/plugin/limiter/meshregistry
  - enable: 是否开启该模块
  - general: `limiter`启动相关参数
    - disableGlobalRateLimit：禁用全局共享限流
    - disableAdaptive: 禁用自适应限流
    - disableInsertGlobalRateLimit: 禁止模块插入全局限流相关的插件
  - global: `limiter`依赖的一些全局参数，global具体参数可参考 [Config.global](#configglobal)

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: limiter
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-limiter
    tag: v0.7.1
  module:
    - name: limiter
      kind: limiter
      enable: true
      general:
        disableGlobalRateLimit: true
        disableAdaptive: true
        disableInsertGlobalRateLimit: true
```

### plugin 安装样例

安装plugin模块

- image: 指定`plugin`的镜像，包括策略，仓库，tag
- module: 指定`plugin`部署的相关参数
  - name: 模块名称
  - kind：模块类别，目前只支持 lazyload/plugin/limiter/meshregistry
  - enable: 是否开启该模块
  - global: `plugin`依赖的一些全局参数，global具体参数可参考 [Config.global](#configglobal)

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: plugin
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-plugin
    tag: v0.7.1
  module:
    - name: plugin
      kind: plugin
      enable: true
```

### meshregistry安装样例

安装对接多注册中心的meshregistry模块

- image: 指定`meshregistry`的镜像，包括策略，仓库，tag
- module: 指定`meshregistry`部署的相关参数
  - name: 模块名称
  - kind：模块类别，目前只支持 lazyload/plugin/limiter/meshregistry
  - enable: 是否开启该模块
  - general: `meshregistry`启动相关参数，支持配置K8SSource、EurekaSource、NacosSource、ZookeeperSource
  - global: `meshregistry`依赖的一些全局参数，global具体参数可参考 [Config.global](#configglobal)

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: meshregistry
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-meshregistry
    tag: v0.7.1
  module:
  - name: meshregistry
    kind: meshregistry
    enable: true
    general:
      LEGACY:
        NacosSource:
          Enabled: true
          RefreshPeriod: 30s
          Address:
          - "http://nacos.test.com"
          Mode: polling
        # EurekaSource:
        #   Enabled: true
        #   Address:
        #   - "http://test/eureka"
        #   RefreshPeriod: 15s
        #   SvcPort: 80
        # ZookeeperSource:
        #   Enabled: true
        #   RefreshPeriod: 30s
        #   WaitTime: 10s
        #   Address:
        #   - zookeeper.test.svc.cluster.local:2181
```

### bundle模式安装样例

在上面的样例中，我们部署了`lazyload`，`limiter`和`plugin`模块，现在我们用`bundle`的模式安装包含上面三个功能的`bundle`模块

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: bundle
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-bundle-all
    tag: v0.7.1
  module:
    - name: bundle
      enable: true
      bundle:
        modules:
          - name: bundle
            kind: lazyload
          - name: limiter
            kind: limiter
          - name: plugin
            kind: plugin
          - name: meshregistry
            kind: meshregistry
      global:
        log:
          logLevel: info
    - name: bundle #与上面的name一致TODO
      kind: lazyload
      enable: true
      mode: BundleItem
      general:
        autoPort: true
        autoFence: true
        defaultFence: true
        wormholePort: # replace to your application service ports, and extend the list in case of multi ports
        - "9080"
      global:
        misc:
          globalSidecarMode: cluster # the mode of global-sidecar
          metricSourceType: accesslog # indicate the metric source
        slimeNamespace: mesh-operator
    - name: limiter
      kind: limiter
      enable: true
      mode: BundleItem
      general:
        disableGlobalRateLimit: true
        disableAdaptive: true
        disableInsertGlobalRateLimit: true
    - name: plugin
      kind: plugin
      enable: true
      mode: BundleItem   
    - name: meshregistry
      kind: meshregistry
      enable: true
      mode: BundleItem
      general:
        LEGACY:
          MeshConfigFile: ""
          RevCrds: ""
          Mcp: {}
          K8SSource:
            Enabled: false   
  component:
    globalSidecar:
      replicas: 1
      enable: true
      sidecarInject:
        enable: true # should be true
        mode: pod
        labels: # optional, used for sidecarInject.mode = pod
          sidecar.istio.io/inject: "true"
      resources:
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 400m
          memory: 400Mi
      image:
        repository: docker.io/slimeio/slime-global-sidecar
        tag: v0.7.1
      probePort: 20000 # health probe port
      port: 80 # global-sidecar default svc port
      legacyFilterName: true
```

### 多副本样例

以limiter为例，安装带有两个副本的limiter模块。

我们需要设置`enableLeaderElection: "on"`以及`replicaCount: 2` 可开启多副本模式。

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: limiter
  namespace: mesh-operator
spec:
  replicaCount: 2   #多副本
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-limiter
    tag: v0.7.1
  module:
    - name: limiter
      kind: limiter
      enable: true
      general:
        disableGlobalRateLimit: true
        disableAdaptive: true
        disableInsertGlobalRateLimit: true
      global:
        misc:
          enableLeaderElection: "on"
```


### Config.global

关于上面涉及到的 Config.global 内容如下：

| Key                            | Default Value                                                                                                                              | Usages                                                                                                                                                                                                                                                                                                                         | Remark |
|--------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------|
| service                        | app                                                                                                                                        | servicefence匹配服务的label key，用来生成懒加载中sidecar的默认配置                                                                                                                                                                                                                                                                                |        |
| istioNamespace                 | istio-system                                                                                                                               | 部署istio组件的namespace，用来生成懒加载中sidecar的默认配置，应等于实际部署istio组件的namespace                                                                                                                                                                                                                                                              |        |
| slimeNamespace                 | mesh-operator                                                                                                                              | 部署slime模块的namespace，用来生成懒加载中sidecar的默认配置，应等于实际创建slimeboot cr资源的namespace                                                                                                                                                                                                                                                       |        |
| log.logLevel                   | ""                                                                                                                                         | slime自身日志级别                                                                                                                                                                                                                                                                                                                    |        |
| log.klogLevel                  | 0                                                                                                                                          | klog日志级别                                                                                                                                                                                                                                                                                                                       |        |
| log.logRotate                  | false                                                                                                                                      | 是否启用日志轮转，即日志输出本地文件                                                                                                                                                                                                                                                                                                             |        |
| log.logRotateConfig.filePath   | "/tmp/log/slime.log"                                                                                                                       | 本地日志文件路径                                                                                                                                                                                                                                                                                                                       |        |
| log.logRotateConfig.maxSizeMB  | 100                                                                                                                                        | 本地日志文件大小上限，单位MB                                                                                                                                                                                                                                                                                                                |        |
| log.logRotateConfig.maxBackups | 10                                                                                                                                         | 本地日志文件个数上限                                                                                                                                                                                                                                                                                                                     |        |
| log.logRotateConfig.maxAgeDay  | 10                                                                                                                                         | 本地日志文件保留时间，单位天                                                                                                                                                                                                                                                                                                                 |        |
| log.logRotateConfig.compress   | false                                                                                                                                      | 本地日志文件轮转后是否压缩                                                                                                                                                                                                                                                                                                                  |        |
| misc                           | {"metrics-addr": ":8080", "aux-addr": ":8081","globalSidecarMode": "namespace","metricSourceType": "prometheus","logSourcePort": ":8082"}, | 可扩展的配置集合，目前支持一下参数参数：1."metrics-addr"定义slime module manager监控指标暴露地址；2."aux-addr"定义辅助服务器暴露地址；3."globalSidecarMode"定义global-sidecar的使用模式，默认是"namespace"，可选的还有"cluster", "no"；4."metricSourceType"定义监控指标来源，默认是"prometheus"，可选"accesslog"；5."logSourcePort"定义使用accesslog做指标源时，接收accesslog的端口，默认是8082，如果要修改，注意也要修改helm模板中的logSourcePort |
| seLabelSelectorKeys            | app                                                                                                                                        | 默认应用标识，se 涉及                                                                                                                                                                                                                                                                                                                   |        |
| xdsSourceEnableIncPush         | true                                                                                                                                       | 是否进行xds增量推送                                                                                                                                                                                                                                                                                                                    |
| pathRedirect                   | ""                                                                                                                                         | path从定向映射表                                                                                                                                                                                                                                                                                                                     |

