- [自适应限流模块](#自适应限流模块)
  - [安装和使用](#安装和使用)
    - [安装Limiter模块](#安装limiter模块)
  - [SmartLimiter](#smartlimiter)
    - [网格场景单机限流](#网格场景单机限流)
    - [网格场景全局均分限流](#网格场景全局均分限流)
    - [网格场景全局共享限流](#网格场景全局共享限流)
    - [网关场景单机限流](#网关场景单机限流)
    - [网关场景全局共享限流](#网关场景全局共享限流)
  - [实践](#实践)
    - [实践1：网格场景全局均分](#实践1网格场景全局均分)
    - [实践2：网格场景全局共享](#实践2网格场景全局共享)
  - [依赖](#依赖)
    - [安装 configmap](#安装-configmap)
    - [安装 Prometheus](#安装-prometheus)
    - [安装 RLS & Redis](#安装-rls--redis)
  - [问题排查](#问题排查)

# 自适应限流模块

## 安装和使用

### 安装Limiter模块

前提：在部署limiter模块前需要安装`CRD`和`deployment/slime-boot`, 参考[slime-boot 安装](https://raw.githubusercontent.com/slime-io/slime/master/doc/zh/slime-boot.md) 指引安装`slime-boot`

在`CRD`和`deployment/slime-boot`安装成功后，用户可手动应用以下yaml清单，安装limiter模块 [limiter](../install/limiter.yaml)， 在该份部署清单中我们只提供了的单机限流和均分限流

如需支持全局共享限流，请阅读[安装 Rls & Redis](#安装-RLS-&-Redis), [安装 configmap](#安装 configmap)

如需要支持自适应限流，请阅读[安装 Prometheus](#安装-prometheus)

## SmartLimiter

smarlimiter 定义见 [proto](https://raw.githubusercontent.com/slime-io/slime/master/staging/src/slime.io/slime/modules/limiter/api/v1alpha2/smart_limiter.proto)

**注意**： 网格场景下，每个服务只能创建一个SmartLimiter资源，其name和namespace对应着服务的service的name和namespace

### 网格场景单机限流

- 入方向限流：服务入方向进行限流，即A->B, 在B段入口方向对请求进行限流判断

以下smartlimiter表示，对reviews服务的入方向9080端口进行限流，限流规则是100次/60秒

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  sets:
    _base: 
      descriptor:   
      - action: 
          fill_interval:
            seconds: 60
          quota: '100'
          strategy: 'single'  
        condition: 'true'
        target:
          port: 9080
```

- 出方向限流：服务出方向进行限流，即A->B, 在A段出口方向对B的请求进行限流判断

以下smartlimiter表示，在productpage服务的出方向限流，即访问路由reviews.default.svc.cluster.local:80/default时进行限流判断，限流规则是2次/10秒

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: productpage
  namespace: default
spec:
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 10
          quota: "2"
          strategy: single
        condition: "true"
        target:
          direction: outbound
          route:
          - reviews.default.svc.cluster.local:80/default
```

- 网格场景带match的限流

在很多场景下，我们希望对带有某个header的请求进行限流

以下smartlimiter表示，对于default下的productpage服务的9080端口，如果请求的header中包含foo=bar, 那么就进行限流，限流规则是 5次/60s

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: productpage
  namespace: default
spec:
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "5"
          strategy: "single"
        condition: "true"
        target:
          port: 9080
        match:
         - name: foo
           exact_match: bar
```

网格场景带match的限流,除了支持Header的精确匹配，还支持正则匹配、前缀匹配、后缀匹配、存在性等功能，具体可参考[SmartLimitDescriptor](../api/v1alpha2/smart_limiter.proto)


### 网格场景全局均分限流

以下smartlimiter表示，对于default下的reviews服务的9080端口，进行限流，限流总数为100次/60s, 按两个pod来说，每个pod限流规则是50次/60s

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: '100/{{._base.pod}}'
          strategy: 'average'  
        condition: 'true'
        target:
          port: 9080
```

### 网格场景全局共享限流

服务的所有pod, 共享一个全局的限流计数器

举个简单的列子，服务A有3个pod, 每次对A的任意pod进行访问都会使A服务全局限流计数器加一，直至触发限流。限流的表现就是有服务访问服务A，服务A将返回429响应码。

以下smartlimiter表示，对于default下的reviews服务的9080端口，进行限流，限流总数为100次/60s, reviews服务的所有pods共享100次/60s

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: '100'
          strategy: 'global'
        condition: 'true'
        target:
          port: 9080            
```

### 网关场景单机限流

网关模式下的限流只作用在出方向

以下smartlimiter表示，对于匹配 `gw_cluster: prod-gateway`的pods，下发单机限流规则，对路由a.test.com:80/r1的访问的限流规则是 1次/60s

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: prod-gateway-1
  namespace: gateway-system
spec:
  gateway: true  # 针对网关的限流配置
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60      # 时间，单位秒  
          quota: "1"         # 个数
          strategy: single
        condition: "true"
        target:
          direction: outbound
          route:
          - a.test.com:80/r1   # host/路由
  workloadSelector:
    gw_cluster: prod-gateway  # 物理网关label,一般不动
```

除了支持route级别，网关单机限流还支持host级别的限流

以下smartlimiter表示，对于匹配 `gw_cluster: prod-gateway`的pods, 下发单机限流规则，对于host b.test.com的访问进行限流，限流规则是1次/60s

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: prod-gateway-2
  namespace: gateway-system
spec:
  gateway: true
  workloadSelector:
    gw_cluster: prod-gateway
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: '2'
          strategy: 'single'
        condition: 'true'
        target:
          direction: outbound
          port: 80
          host:
          - b.test.com
```

### 网关场景全局共享限流

以下smartlimiter表示，对于匹配 `gw_cluster: prod-gateway`的pods, 下发全局共享限流规则，对于路由a.test.com:80/default的访问进行限流，限流规则是10次/60s

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: prod-gateway-3
  namespace: gateway-system
spec:
  gateway: true
  workloadSelector:
    gw_cluster: prod-gateway 
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "10"
          strategy: global
        condition: "true"
        match:
        target:
          direction: outbound
          route:
          - a.test.com:80/default
```

## 实践

### 实践1：网格场景全局均分

首先以下SmartLimiter资源到集群, 对productpage服务进行每分钟均分10次限流。

~~~yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: productpage
  namespace: default
spec:
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: '10/{{._base.pod}}'
          strategy: 'average'  
        condition: 'true'
        target:
          port: 9080
~~~

然后SmartLimiter资源（如下，部分内容被移除），在SmartLimiter中，多出metricStatus和ratelimitStatus字段，可以看到由于productpage的副本数只有1，所有quota的值为10/1 = 10。

~~~yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: productpage
  namespace: default
spec:
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: 10/{{._base.pod}}
          strategy: average
        condition: "true"
        target:
          port: 9080
status:
  metricStatus:
    _base.pod: "1"
  ratelimitStatus:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "10"
          strategy: average
        target:
          port: 9080
~~~

最后，我们通过curl的方式去访问 productpage 服务，在访问第11次的时候出现了429 Too Many Requests，也就是服务被限流了，这也就证实了服务的有效性。

~~~
...
...
... 省略

node@ratings-v1-85b8d86597-smv5m:/opt/microservices$ curl -I http://productpage:9080/productpage
HTTP/1.1 200 OK
content-type: text/html; charset=utf-8
content-length: 3769
server: envoy
date: Fri, 26 Nov 2021 07:16:21 GMT
x-envoy-upstream-service-time: 9

node@ratings-v1-85b8d86597-smv5m:/opt/microservices$ curl -I http://productpage:9080/productpage
HTTP/1.1 429 Too Many Requests
content-length: 18
content-type: text/plain
date: Fri, 26 Nov 2021 07:16:21 GMT
server: envoy
x-envoy-upstream-service-time: 10
~~~

以下对照生成的EnvoyFilter清单，解释更底层的细节。

EnvoyFilter清单包含3个patch部分，分别是给流量生成了genericKey、开启envoy.filters.http.local_ratelimit 插件功能，以及bucket的设置。

~~~yaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  creationTimestamp: "2021-11-26T07:09:58Z"
  generation: 1
  name: productpage.default.ratelimit
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha2
    blockOwnerDeletion: true
    controller: true
    kind: SmartLimiter
    name: productpage
spec:
  configPatches:
  - applyTo: HTTP_ROUTE
    match:
      routeConfiguration:
        vhost:
          name: inbound|http|9080
          route:
            name: default
    patch:
      operation: MERGE
      value:
        route:
          rate_limits:
          - actions:
            - generic_key:
                descriptor_value: Service[productpage.default]-User[none]-Id[3651085651]
  - applyTo: HTTP_FILTER
    match:
      context: SIDECAR_INBOUND
      listener:
        filterChain:
          filter:
            name: envoy.http_connection_manager
            subFilter:
              name: envoy.router
    patch:
      operation: INSERT_BEFORE
      value:
        name: envoy.filters.http.local_ratelimit
        typed_config:
          '@type': type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
          stat_prefix: http_local_rate_limiter
  - applyTo: HTTP_ROUTE
    match:
      routeConfiguration:
        vhost:
          name: inbound|http|9080
          route:
            name: default
    patch:
      operation: MERGE
      value:
        typed_per_filter_config:
          envoy.filters.http.local_ratelimit:
            '@type': type.googleapis.com/udpa.type.v1.TypedStruct
            type_url: type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
            value:
              descriptors:
              - entries:
                - key: generic_key
                  value: Service[productpage.default]-User[none]-Id[3651085651]
                token_bucket:
                  fill_interval:
                    seconds: "60"
                  max_tokens: 10
                  tokens_per_fill:
                    value: 10
              filter_enabled:
                default_value:
                  numerator: 100
                runtime_key: local_rate_limit_enabled
              filter_enforced:
                default_value:
                  numerator: 100
                runtime_key: local_rate_limit_enforced
              stat_prefix: http_local_rate_limiter
              token_bucket:
                fill_interval:
                  seconds: "1"
                max_tokens: 100000
                tokens_per_fill:
                  value: 100000
  workloadSelector:
    labels:
      app: productpage
~~~

### 实践2：网格场景全局共享

提交以下资源到集群, 对productpage服务进行每分钟共计10次限流。

~~~yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: productpage
  namespace: default
spec:
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "10"
          strategy: "global"
        condition: "true"
        target:
          port: 9080       
~~~

然后查看提交的SmartLimiter资源（如下，部分内容被移除）

~~~yaml
kind: SmartLimiter
metadata:
  name: productpage
  namespace: default
spec:
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "10"
          strategy: global
        condition: "true"
        target:
          port: 9080
status:
  metricStatus:
    _base.cpu.max: "563.3837582"
    _base.pod: "1"
  ratelimitStatus:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "10"
          strategy: global
        target:
          port: 9080
~~~

最后，我们通过curl的方式去访问 productpage 服务，在访问第11次的时候出现了429 Too Many Requests，也就是服务被限流了，这也就证实了服务的有效性。

~~~
...
...
... 省略

node@ratings-v1-85b8d86597-smv5m:/opt/microservices$ curl -I http://productpage:9080/productpage
HTTP/1.1 200 OK
content-type: text/html; charset=utf-8
content-length: 3769
server: envoy
date: Fri, 26 Nov 2021 07:33:11 GMT
x-envoy-upstream-service-time: 10

node@ratings-v1-85b8d86597-smv5m:/opt/microservices$ curl -I http://productpage:9080/productpage
HTTP/1.1 429 Too Many Requests
x-envoy-ratelimited: true
date: Fri, 26 Nov 2021 07:33:11 GMT
server: envoy
x-envoy-upstream-service-time: 1
transfer-encoding: chunked
~~~

以下对照生成的EnvoyFilter和ConfigMap清单，解释更底层的细节。

在EnvoyFilter中包含两个patch,一个是给流量生成了genericKey，另一个是设置RLS的服务地址outbound|18081||rate-limit.istio-system.svc.cluster.local

~~~
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  creationTimestamp: "2021-11-26T07:31:31Z"
  name: productpage.default.ratelimit
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha2
    blockOwnerDeletion: true
    controller: true
    kind: SmartLimiter
    name: productpage
spec:
  configPatches:
  - applyTo: HTTP_ROUTE
    match:
      routeConfiguration:
        vhost:
          name: inbound|http|9080
          route:
            name: default
    patch:
      operation: MERGE
      value:
        route:
          rate_limits:
          - actions:
            - generic_key:
                descriptor_value: Service[productpage.default]-User[none]-Id[2970232041]
  - applyTo: HTTP_FILTER
    match:
      context: SIDECAR_INBOUND
      listener:
        filterChain:
          filter:
            name: envoy.http_connection_manager
            subFilter:
              name: envoy.router
    patch:
      operation: INSERT_BEFORE
      value:
        name: envoy.filters.http.ratelimit
        typed_config:
          '@type': type.googleapis.com/udpa.type.v1.TypedStruct
          type_url: type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit
          value:
            domain: slime
            rate_limit_service:
              grpc_service:
                envoy_grpc:
                  cluster_name: outbound|18081||rate-limit.istio-system.svc.cluster.local
              transport_api_version: V3
  workloadSelector:
    labels:
      app: productpage
~~~

ConfigMap内容中包含了一个config.yaml，其内容是将要挂载到RLS的/data/ratelimit/config目录，对带有Service[productpage.default]-User[none]-Id[2970232041]的流量进行限流10/min.

~~~
apiVersion: v1
data:
  config.yaml: |
    domain: slime
    descriptors:
    - key: generic_key
      value: Service[productpage.default]-User[none]-Id[2970232041]
      rate_limit:
        requests_per_unit: 10
        unit: MINUTE
kind: ConfigMap
metadata:
  name: rate-limit-config
  namespace: istio-system
~~~

## 依赖

### 安装 configmap

全局共享限流的实现，依赖rls服务，同时也需要将限流规则挂载至rsl服务，所以需要提前安装rate-limit-config

资源内容如下 [mesh](../install/rate-limit-config.yaml)

如果是网关场景，需要下发以下资源 [gateway](../install/rate-limit-config-gw.yaml)


### 安装 Prometheus

Prometheus 是一款广泛应用的监控系统，本服务依赖prometheus采集服务指标。

为此我们提供了一个简单的Prometheus安装清单，使用以下命令进行安装。

```
kubectl apply -f "https://raw.githubusercontent.com/slime-io/limiter/master/install/prometheus.yaml"
```

### 安装 RLS & Redis

RLS服务即 Rate Limit Service [RLS](https://github.com/envoyproxy/ratelimit) , 我们利用它支持全局共享限流, 如果确认服务不需要支持全局限流可以选择不安装Rls, 跳过该小节。

简单介绍下RLS服务，它是一个GO开发的gRPC服务，利用Redis支持了全局限流的功能。当你配置了全局限流SmartLimiter后，该资源清单首先会被转化成EnvoyFilter，Istio会根据EnvoyFilter内容下发限流规则到相应的Envoy，之后Envoy在执行全局限流规则时，会去访问RLS服务，让其决定是否进行限流。

为此我们提供了一个简单的 RLS&Redis 安装清单，使用以下命令进行安装。

~~~shell
kubectl apply -f "https://raw.githubusercontent.com/slime-io/limiter/master/install/rls.yaml"
~~~

如果是网关场景，需要修改以上资源，并部署到网关所处的ns




## 问题排查

如果出现限流未生效的情况，可以顺着以下思路进行排查。

1. Limiter 日志是否出现异常
2. EnvoyFilter或者ConfigMap是否正常生成（全局限流）
3. 通过config dump 命令查看envoy限流配置是否真实生效
4. RLS服务的 /data/ratelimit/config 目录下是否有相关的ConfigMap内容（全局限流）



欢迎交流~~~
