- [Introduction and use of SlimeBoot](#introduction-and-use-of-slimeboot)
  - [Introduction](#introduction)
  - [Preparation](#preparation)
  - [Introduction of parameters](#introduction-of-parameters)
  - [Installation](#installation)
    - [Sample lazyload installation](#sample-lazyload-installation)
    - [Sample limiter installation](#sample-limiter-installation)
    - [Sample plugin installation](#sample-plugin-installation)
    - [Sample bundle mode installation](#sample-bundle-mode-installation)
    - [Config.global](#configglobal)


[中文](../zh/slime-boot.md)

# Introduction and use of SlimeBoot

## Introduction

This article will introduce the use of `SlimeBoot` and give you a sample usage to guide you to install and use the `slime` component. `slime-boot` can be understood as a `Controller`, it will always listen to the `SlimeBoot CR`, when the user submits a `SlimeBoot CR`, the `slime-boot Controller` will render the `slime` related deployment material according to the `CR` content.

## Preparation

Before installing the `slime` component, you need to install `SlimeBoot CRD` and `deployment/slime-boot`

**Note**: In k8s v1.22 and later, only the `apiextensions.k8s.io/v1` version of `CRD` is supported, and the `apiextensions.k8s.io/v1beta1` version of `CRD` is no longer supported, see the [k8s official documentation](https://) kubernetes.io/docs/reference/using-api/deprecation-guide/#customresourcedefinition-v122), and k8s v1.16~v1.21 both versions can be used

1. For k8s v1.22 and later, you need to manually install [v1] (... /... /install/init/crds-v1.yaml), while the previous version can be installed manually (... /... /... /install/init/crds-v1.yaml) or [v1beta1] (... /... /install/init/crds.yaml)
2. manually install [deployment/slime-boot](. /... /install/init/deployment_slime-boot.yaml)

Or install `CRD` and `deployment/slime-boot` by executing the following commands, note that if the network is not accessible, you can find the relevant documentation in the following directory `slime/install/init/`


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

## Introduction of parameters
According to the previous section, we know that the user installs the `slime` component by distributing `SlimeBoot`. In normal use, the user uses the `SlimeBoot CR` which contains the following parameters

- image: defines the parameters related to the image
- resources: defines the container resources
- module: defines the module to be started, and the corresponding parameters
  - name: the name of the module
  - kind: the category of the module, currently only lazyload/plugin/limiter are supported
  - enable: whether to enable the module or not
  - global: some global parameters that the module depends on, see [Config.global](#configglobal) for more details
  - general: some parameters needed to start the module

Examples are as follows

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

## Installation

`SlimeBoot` supports two types of installation

- Module deployment: you need to deploy a copy of `deployment` for each submodule

The following yaml is an incomplete example of a module deployment, the first object defined in the module is the functionality that `SlimeBoot` should have

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

- bundle deployment: just deploy a `deployment`, the `deployment` contains multiple component functions

The following yaml is an incomplete example of the bundle pattern, where the first object of the module defines that the service has limiter and plugin functions, and the last two objects in the mudule correspond to the specific parameters of the limiter and plugin submodules, respectively

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
    tag: xx
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


The following will install the `lazyload`, `limiter` and `plugin` modules in a modular way and the `bundle` module in `bundle` mode


### Sample lazyload installation

Deployment supports lazyload module at `cluster` level, after successful deployment `deployment` named `lazyload` and `global-sidecar` under `mesh-operator` namespace

- istioNamespace: the `ns` of the `istio` deployment in the user cluster
- module: specifies the parameters for `lazyload` deployment
  - name: the name of the module
  - kind: the category of the module, currently only lazyload/plugin/limiter is supported
  - enable: whether to enable the module
  - general: `lazyload` startup related parameters
  - global: some global parameters that `lazyload` depends on, see [Config.global](#configglobal)
  - metric: information about the metrics that `lazyload` depends on for the electrophoretic relationship between services
- component: the configuration of `globalSidecar` in the lazyload module, which is generally not changed except for mirroring

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
        autoPort: false
        autoFence: true
        defaultFence: false   
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
        # mode definition:
        # "pod": sidecar auto-inject on pod level, need provide labels for injection
        # "namespace": sidecar auto-inject on namespace level, no need to provide labels for injection
        # if globalSidecarMode is cluster, global-sidecar will be deployed in slime namespace, which does not enable auto-inject on namespace level, mode can only be "pod".
        # if globalSidecarMode is namespace, depending on the namespace definition, mode can be "pod" or "namespace".
        mode: pod
        labels: # optional, used for sidecarInject.mode = pod
          sidecar.istio.io/inject: "true"
          # istio.io/rev: canary # use control plane revisions
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

### Sample limiter installation

Install a limiter module that supports single-computer limiting, and deploy a `deployment` named `limiter` under the `mesh-operator` namespace after success

- image: Specify the image of `limiter`, including policy, repository, tag
- module: Specifies the parameters for `limiter` deployment
  - name: the name of the module
  - kind: module category, currently only supports lazyload/plugin/limiter
  - enable: whether to enable the module
  - general: `limiter` startup related parameters
    - disableGlobalRateLimit: disable global shared limiting
    - disableAdaptive: Disable adaptive limiting
    - disableInsertGlobalRateLimit: disables the module from inserting global flow-limiting related plugins
  - global: Some global parameters that `limiter` depends on. global specific parameters can be found in [Config.global](#configglobal)

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

### Sample plugin installation

Install the plugin module

- image: Specify the `limiter` image, including policy, repository, tag
- module: Specify the parameters for `limiter` deployment
  - name: module name
  - kind: module category, currently only supports lazyload/plugin/limiter
  - enable: whether to enable the module
  - global: some global parameters that `plugin` depends on, global specific parameters can be found in [Config.global](#configglobal)


```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: limiter
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

### Sample bundle mode installation

In the above example, we deployed `lazyload`, `limiter` and `plugin` modules, now we install the `bundle` module with the above three functions in `bundle` mode


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
          - name: lazyload
            kind: lazyload
          - name: limiter
            kind: limiter
          - name: plugin
            kind: plugin            
      global:
        log:
          logLevel: info
    - name: lazyload
      kind: lazyload
      enable: true
      mode: BundleItem
      general:
        autoPort: false
        autoFence: true
        defaultFence: false
        wormholePort: # replace to your application service ports, and extend the list in case of multi ports
        - "9080"
      global:
        misc:
          globalSidecarMode: cluster # the mode of global-sidecar
          metricSourceType: accesslog # indicate the metric source
        slimeNamespace: mesh-operator
        log:
          logLevel: info
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
  component:
    globalSidecar:
      replicas: 1
      enable: true
      sidecarInject:
        enable: true # should be true
        # mode definition:
        # "pod": sidecar auto-inject on pod level, need provide labels for injection
        # "namespace": sidecar auto-inject on namespace level, no need to provide labels for injection
        # if globalSidecarMode is cluster, global-sidecar will be deployed in slime namespace, which does not enable auto-inject on namespace level, mode can only be "pod".
        # if globalSidecarMode is namespace, depending on the namespace definition, mode can be "pod" or "namespace".
        mode: pod
      resources:
        limits:
          cpu: 2000m
          memory: 2048Mi
        requests:
          cpu: 1000m
          memory: 1024Mi
      image:
        repository: docker.io/slimeio/slime-global-sidecar
        tag: v0.7.1
      probePort: 20000 # health probe port
      port: 80 # global-sidecar default svc port
      legacyFilterName: true
```

### Config.global

Regarding the Config.global involved above, the content is as follows.

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