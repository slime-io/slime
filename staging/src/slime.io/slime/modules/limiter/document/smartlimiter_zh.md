- [自适应限流](#自适应限流)
  - [安装和使用](#安装和使用)
    - [安装 Prometheus](#安装-prometheus)
    - [安装 Rls & Redis](#安装-RLS-&-Redis)
    - [安装 Limiter](#安装-limiter)
  - [SmartLimiter](#smartlimiter)
    - [单机限流](#单机限流)
    - [全局均分限流](#全局均分限流)
    - [全局共享限流](#全局共享限流)
  - [实践](#实践)
    - [实践1：全局均分](#实践1全局均分)
    - [实践2：全局共享](#实践2全局共享)
  - [问题排查](#问题排查)
# 自适应限流

## 安装和使用

在安装服务前请先阅读  [安装Prometheus](#安装-prometheus) 和 [安装 Rls & Redis](#安装-rls-&-redis)

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

~~~
kubectl apply -f "https://raw.githubusercontent.com/slime-io/limiter/master/install/rls.yaml"
~~~

### 安装 Limiter

请先按照[slime-boot 安装](https://raw.githubusercontent.com/slime-io/slime/master/doc/zh/slime-boot.md) 指引安装`slime-boot`，该服务是slime的一个引导程序，安装后用户可以通过提交SlimeBoot资源的方式安装不同的slime模块。

之后，用户可以手动提交以下yaml 文件，安装 limiter模块，该SlimeBoot可以指定limiter镜像版本以及需要查询的具体指标。

为此我们提供了一个简单的SlimeBoot安装清单，使用以下命令进行安装。

```
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime-io/limiter/master/install/limiter.yaml"
```

清单的具体内容如下

```yaml
---
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: smartlimiter
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-limiter
    tag: v0.2.0_linux_amd64
  module:
    - name: limiter # custom value
      kind: limiter # should be "limiter"
      enable: true
      general: # replace previous "limiter" field
        backend: 1
      metric:
        prometheus:
          address: http://prometheus.istio-system:9090
          handlers:
            cpu.sum:
              query: |
                sum(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""})
            cpu.max:
              query: |
                max(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""})
            rt99:
              query: |
                histogram_quantile(0.99, sum(rate(istio_request_duration_milliseconds_bucket{kubernetes_pod_name=~"$pod_name"}[2m]))by(le))
        k8s:
          handlers:
            - pod # inline
```

在上面清单中，我们默认配置了prometheus作为监控源，prometheus.handlers定义了希望从监控中获取的监控指标，这些监控指标可以作为一些自适应算法的阈值，从而达到自适应限流。

用户也可以根据需要定义limiter模块需要获取的监控指标，以下是一些可以常用的监控指标获取语句：

```
cpu:
总和：
sum(rate(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""} [5m])) by（container_name）
最大值：
max(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""})
limit:
container_spec_cpu_quota{pod=~"$pod_name"}

内存：
总和：
sum(container_memory_usage_bytes{namespace="$namespace",pod=~"$pod_name",image=""} [5m])
最大值：
max(container_memory_usage_bytes{namespace="$namespace",pod=~"$pod_name",image=""})
limit:
sum(container_spec_memory_limit_bytes{pod=~"$pod_name"})

请求时延：
90值：
histogram_quantile(0.90, sum(rate(istio_request_duration_milliseconds_bucket{kubernetes_pod_name=~"$pod_name"}[2m]))by(le))
95值：
histogram_quantile(0.95, sum(rate(istio_request_duration_milliseconds_bucket{kubernetes_pod_name=~"$pod_name"}[2m]))by(le))
99值：
histogram_quantile(0.99, sum(rate(istio_request_duration_milliseconds_bucket{kubernetes_pod_name=~"$pod_name"}[2m]))by(le))
```

## SmartLimiter 

定义见 [proto](https://raw.githubusercontent.com/slime-io/slime/master/staging/src/slime.io/slime/modules/limiter/api/v1alpha2/smart_limiter.proto)

注意每个服务只能创建一个SmartLimiter资源，其name和namespace对应着service的name和namespace

### 单机限流

单机限流功能替服务的每个pod设置固定的限流数值，其底层是依赖envoy插件envoy.filters.http.local_ratelimit 提供的限流能力，[Local Ratelimit Plugin](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/local_rate_limit_filter)。

简单样例如下，我们对reviews服务进行限流，根据condition字段的值判断是否执行限流，这里我们直接设置了true，让其永久执行限流，同样用户可以设置一个动态的值，limiter 会计算其结果，动态的进行限流。fill_interval 指定限流间隔为60s，quota指定限流数量100，strategy标识该限流是单机限流single，target 字段标识需要限流的端口9080。

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  sets:
    _base:   # 匹配所有服务，关键词 _base ，也可以是你定义的 subset ，如 v1 
      descriptor:   
      - action:    # 限流规则
          fill_interval:
            seconds: 60
          quota: '100'
          strategy: 'single'  
        condition: 'true'  # 永远执行该限流
        #condition: '{{._base.cpu.sum}}>100'  如果服务的所有负载大于100，则执行该限流
        target:
          port: 9080
```

### 全局均分限流

全局均分限功能根据用户设置的总的限流数，然后平均分配到各个pod，底层同样是依赖envoy插件envoy.filters.http.local_ratelimit 提供的限流能力[Local Ratelimit Plugin](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/local_rate_limit_filter)。

简单样例如下，我们对reviews服务进行限流，

根据condition字段的值判断是否执行限流，这里我们直接设置了true，让其永久执行限流，同样用户可以设置一个动态的值，limiter 会计算其结果，动态的进行限流。fill_interval 指定限流间隔为60s，quota指定限流数量100/{{._base.pod}}, {{._base.pod}}的值是由limiter模块根据metric计算得到，假如该服务有2个副本，那么quota的值为50，strategy标识该限流是均分限流，target 字段标识需要限流的端口9080。

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

### 全局共享限流

全局共享限流功能替服务的所有pod维护了一个全局计数器，底层依赖的是envoy插件nvoy.filters.http.ratelimit 提供的限流能力 [Ratelimit Plugin](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rate_limit_filter) 和RLS服务提供给的全局计数能力[RLS](https://github.com/envoyproxy/ratelimit) 。

当提交一个全局共享限流SmartLimiter后，limiter模块会根据其内容生成EnvoyFilter和名为slime-rate-limit-config的ConfigMap。EnvoyFilter会被Istio监听到，下发限流配置至envoy，而ConfigMap则会被挂载到RLS服务，RLS根据ConfigMap内容生成全局共享计数器。

简单样例如下，我们对reviews服务进行限流，字段含义可参考上面文档。主要区别在于 strategy为global，并且有rls 地址，如果不指定的话为默认为outbound|18081||rate-limit.istio-system.svc.cluster.local，这对应着默认安装的RLS。注意：由于RLS功能的要求，seconds 只支持 1、60、3600、86400，即1秒、1分钟、1小时、1天

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  sets:
    _base:
      #rls: 'outbound|18081||rate-limit.istio-system.svc.cluster.local' 如果不指定默认是该地址
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

## 实践

为bookinfo的productpage服务开启自适应限流功能。

注意每个服务只能创建一个SmartLimiter资源，其name和namespace对应着service的name和namespace

安装 istio (1.8+)

### 实践1：全局均分

首先提交一份全局均分SmartLimiter资源到集群, 对productpage服务进行每分钟均分10次限流。

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

然后我们查看提交的SmartLimiter资源（如下，部分内容被移除），在查询到的SmartLimiter中，将会多出metricStatus 和 ratelimitStatus 内容，它们分别代表着这一刻从prometheus中获取的metric，以及当前生效的限流规则。可以看到由于productpage的副本数只有1，所有quota的值为10/1 = 10。

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
    _base.cpu.max: "531.8215157"
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

EnvoyFilter清单包含3个patch部分，分别是给流量生成了genericKey，开启envoy.filters.http.local_ratelimit 插件功能，以及bucket的设置。

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

### 实践2：全局共享

首先提交一份全局共享SmartLimiter资源到集群, 对productpage服务进行每分钟共计10次限流。

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

然后我们查看提交的SmartLimiter资源（如下，部分内容被移除），在查询到的SmartLimiter中，将会多出了metricStatus 和 ratelimitStatus 内容，它们分别代表着这一刻从prometheus中获取的metric.

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
  name: slime-rate-limit-config
  namespace: istio-system
~~~

## 问题排查

如果出现限流未生效的情况，可以顺着以下思路进行排查。

1. Limiter 日志是否出现异常
2. EnvoyFilter或者ConfigMap是否正常生成（全局限流）
3. 通过config dump 命令查看envoy限流配置是否真实生效
4. RLS服务的 /data/ratelimit/config 目录下是否有相关的ConfigMap内容（全局限流）



欢迎交流~~~

