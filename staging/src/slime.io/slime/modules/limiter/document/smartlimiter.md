- [Adaptive rate limiting](#adaptive-rate-limiting)
  - [Installation and Usage](#installation-and-usage)
    - [Installing Prometheus](#installing-prometheus)
    - [Installing Rls & Redis](#installing-rls--redis)
    - [Install Limiter](#install-limiter)
  - [SmartLimiter](#smartlimiter)
    - [Single Ratelimit](#single-ratelimit)
    - [Global Average Ratelimit](#global-average-ratelimit)
    - [Global Shared Ratelimit](#global-shared-ratelimit)
  - [Example](#example)
    - [global average ratelimit](#global-average-ratelimit-1)
    - [global shared ratelimit](#global-shared-ratelimit-1)
  - [Troubleshooting](#troubleshooting)
# Adaptive rate limiting

## Installation and Usage

Please read the Installing [Prometheus](#installing-prometheus) and Installing [RLS & Redis](#installing-rls--redis) subsections before installing the service

### Installing Prometheus 

Prometheus is a widely used monitoring system and limiter relies on prometheus to collect metrics.

A simple Prometheus installation checklist is provided for this purpose, use the following command to install it.

```
kubectl apply -f "https://raw.githubusercontent.com/slime-io/limiter/master/install/prometheus.yaml"
```

### Installing RLS & Redis

RLS service is Rate Limit Service [RLS](https://github.com/envoyproxy/ratelimit), we use it to support global shared rate limiting, if you confirm that the service does not need to support global rate limiting you can choose not to install RLS, skip this section.

A brief introduction to the RLS service is that it is a GO-developed gRPC service that leverages Redis to support fully restricted streaming. After you have configured the SmartLimiter, the resource list will first be transformed into EnvoyFilter, and Istio will send rate limiting restriction rules to the corresponding envoy based on the EnvoyFilter content. 

A simple RLS & Redis installation checklist is provided for this purpose, using the following command

~~~
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/master/install/rls.yaml"
~~~

### Install Limiter

Please first follow the [slime-boot installation](https://raw.githubusercontent.com/slime-io/slime/master/doc/en/slime-boot.md) guidelines to install slime-boot which is a bootloader for slime. After installation, users can install different slime modules by submitting SlimeBoot resources.

The user can manually submit the following yaml file to install the limiter module, which SlimeBoot can specify the limiter image version and the specific metrics to be queried.

A simple SlimeBoot installation checklist is provided for this purpose, using the following command.

```
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime-io/limiter/master/install/limiter.yaml"
```

The details of the list are as follows

```yaml
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

In the above list, we have configured prometheus as the monitoring source by default. prometheus.handlers defines the monitoring metrics that we want to get from prometheus , which can be used as thresholds for some adaptive algorithms to achieve adaptive rate limiting.

 The user can also define the metrics that the limiter module needs to get according to their needs, the following are some of the commonly used statements that can be used to get monitoring metrics.

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

Definition in [proto](https://raw.githubusercontent.com/slime-io/slime/master/staging/src/slime.io/slime/modules/limiter/master/api/v1alpha2/smart_limiter.proto)

Note that each service can only create one SmartLimiter resource, whose name and namespace corresponds to the service's name and namespace

### Single Ratelimit

The single  rate limiting feature sets a fixed rate limiting value for each pod of the service, which relies on the rate limiting capability provided by the envoy plugin envoy.filters.http.local_ratelimit, [Local Ratelimit Plugin](https://www.envoyproxy.io/ docs/envoy/latest/configuration/http/http_filters/local_rate_limit_filter).

A simple example is that we limit reviews service's request, based on the value of the condition field to determine whether to rate limit, here we directly set true, so that it will perform the rate limit permanently, also the user can set a dynamic value, the limiter will calculate the results, execute dynamic rate limit. fill_interval specifies the flow limit interval to 60s, quota specifies the number of limits 10, strategy identifies the limit is a single limit , target field specifies port 9080.

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  sets:
    _base:   # _base match the service
      descriptor:   
      - action:    
          fill_interval:
            seconds: 60
          quota: '10'
          strategy: 'single'  
        condition: 'true' 
        #condition: '{{._base.cpu.sum}}>100'
        target:
          port: 9080
```

### Global Average Ratelimit

The global average ratelimit feature is based on the total number of ratelimits set by the user and then distributed equally to each pod, relying on the ratelimit capability provided by the envoy plugin envoy.filters.http.local_ratelimit [Local Ratelimit Plugin](https://www. envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/local_rate_limit_filter).

For a simple example, let's limit the request to reviews service's .

Here we set true directly to make it permanent, the user can set a dynamic value and the limiter will calculate the result and limit the flow dynamically. fill_interval specifies a limit interval of 60s and quota specifies a limit number of 100/{{. _base.pod}}, The value of {{{._base.pod}} is calculated by the limiter module based on the metric, if the service has 2  pods, then the value of quota is 100/2=50, the strategy field specify to average.

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

### Global Shared Ratelimit

The global shared ratelimit feature maintains a global counter for all pods of the service, relying on the rate limiting capability provided by the envoy plugin nvoy.filters.http.ratelimit [Ratelimit Plugin](https://www.envoyproxy.io/docs/envoy/ latest/configuration/http/http_filters/rate_limit_filter) and the global count capability [RLS](https://github.com/envoyproxy/ratelimit) provided by the RLS service.

When a global shared rate limiting SmartLimiter is submitted, the limiter module generates EnvoyFilter and a ConfigMap named slime-rate-limit-config based on its contents. EnvoyFilter is watched by Istio and sended  to envoy. ConfigMap is mounted to the RLS service, which generates a global shared counter based on the ConfigMap content.

For a simple example, we execute rate limiting on reviews service, and the meaning of the fields can be found in the above document. The main difference is that the strategy is specified to global and RLS address is speccified, if field rls not specified then the default is outbound|18081||rate-limit.istio-system.svc.cluster.local, which corresponds to the default installed RLS.

 note: due to the requirements of the RLS feature seconds only supports 1, 60, 3600, 86400, i.e. 1 second, 1 minute, 1 hour, 1 day

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

## Example

Enable rate limiting for bookinfo's productpage service.

Note that each service can only create one SmartLimiter resource, whose name and namespace corresponds to the service's name and namespace

Install istio (1.8+)

### global average ratelimit

First, submit a global average SmartLimiter resources to the cluster,  which means to limit the productpage service to 10/min

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

Then we qeury the submitted SmartLimiter resource (below, part of the content is removed), in the queried SmartLimiter, there will be more metricStatus and ratelimitStatus contents, which represent the metric obtained from prometheus at this moment, and the currently effective The ratelimit rule is currently in effect. You can see that since the number of pod of the productpage is only 1, the value of all quotas is 10/1 = 10.

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

Finally, we accessed the productpage service by way of curl, and on the 11th visit '429 Too Many Requests' appeared, which means that the service was restricted, which confirms the validity of the service.

~~~
...
...
... 
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

The following explains the lower level details against the generated EnvoyFilter manifest.

EnvoyFilter manifest contains 3 patch parts, which are generating genericKey for traffic, enabling envoy.filters.http.local_ratelimit plugin function, and set bucket.

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

### global shared ratelimit

First, submit a global shared SmartLimiter resource to the cluster, and restrict the productpage service to a total of 10 flows per minute.

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

there will be more metricStatus and ratelimitStatus contents, which represent the metric obtained from prometheus at this moment, and the currently effective The ratelimit rule is currently in effect. 

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

Finally, we accessed the productpage service by way of curl, and on the 11th visit '429 Too Many Requests' appeared, which means that the service was restricted, which confirms the validity of the service.

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

The following explains the lower level details against the generated EnvoyFilter manifest.

EnvoyFilter contains two patches, one is to generate a genericKey for the traffic, the other is to set the RLS service address outbound|18081||rate-limit.istio-system.svc.cluster.local

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

The ConfigMap content contains a config.yaml that will be mounted to the /data/ratelimit/config directory of the RLS and will limit the traffic with Service[productpage.default]-User[none]-Id[2970232041] to Limit the traffic 10/min.

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

## Troubleshooting

If the rate limiting does not take effect, you can troubleshoot along the following lines

1. whether the Limiter log is abnormal
2. whether EnvoyFilter or ConfigMap is generated normally 
3. use config dump command to see if the envoy rate limit configuration is really in effect
4. whether there is any relevant ConfigMap in the /data/ratelimit/config directory of RLS service (global shared ratelimit)

