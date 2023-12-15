## 背景

slime目前在v0.7.2及之前不支持grpc懒加载, 如果grpc服务注入envoy并被纳管到目前的懒加载体系中，会造成请求失败。具体原因是：之前没有考虑h2, grpc协议。

## 方案

用envoy+envoy模式代替之前的envoy+global-sidecar模式,支持grp懒加载。

在该模式下，第一个envoy作为注入的容器，用于获取控制面xds信息，第二个envoy作为业务容器，只进行流量转发。

## 使用

目前该特性需要在slimeboot中手动开启。

**如未开启，默认使用global-sidecar模式，不支持grpc懒加载。**

1. 设置lazyload镜像 >= v0.8.0
2. 设置`supportH2: true`
3. 设置slime-global-sidecar为experimental版本

支持grpc懒加载的完整SlimeBoot配置如下, 注意镜像部分需替换:

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
    tag: v0.9.0 # 1
  namespace: mesh-operator
  istioNamespace: istio-system
  module:
    - name: lazyload
      kind: lazyload
      enable: true
      general:
        supportH2: true  # 2
        autoPort: true
        autoFence: true
        defaultFence: true
        wormholePort:
          - "9080"
        globalSidecarMode: cluster
        metricSourceType: accesslog
      global:
        log:
          logLevel: info
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
        enable: true
        mode: pod
        labels:
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
        tag: v0.8.0-experimental # 3 experimental version
      probePort: 20000
```


## grpc的支持

grpc其实分为四种模式，其中只有简单模式是非流式（一次性流, 而客户端流模式，服务端流模式，双向流模式都属于流式（持久流）。

我们的方案**能够支持grpc简单模式**，即首次访问走gs, 接下来访问走正常路由

对于其他模式的grpc，我们只能做到首次（流）走gs，完成依赖关系建立之后（的流）走正常路由