- [smartlimiter](#smartlimiter)
  - [Installation and use](#installation-and-use)
    - [Installing the Limiter module](#installing-the-limiter-module)
  - [SmartLimiter](#smartlimiter-1)
    - [single smartlimiter in mesh](#single-smartlimiter-in-mesh)
    - [global average smartlimiter in mesh](#global-average-smartlimiter-in-mesh)
    - [global share smartlimiter in mesh](#global-share-smartlimiter-in-mesh)
    - [single smartlimiter in gw](#single-smartlimiter-in-gw)
    - [global share smartlimiter in gw](#global-share-smartlimiter-in-gw)
  - [Practices](#practices)
    - [Practice 1: global average smartlimiter in mesh](#practice-1-global-average-smartlimiter-in-mesh)
    - [Practice 2: global share smartlimiter in mesh](#practice-2-global-share-smartlimiter-in-mesh)
  - [Dependencies](#dependencies)
    - [install Prometheus](#install-prometheus)
    - [install RLS](#install-rls)


# smartlimiter

## Installation and use

### Installing the Limiter module

Prerequisite: `CRD` and `deployment/slime-boot` need to be installed before deploying the limiter module, refer to [slime-boot installation](../../../../../../../doc/en/slime-boot.md#Preparation) for instructions on installing the `SlimeBoot CRD` and `deployment/slime-boot`.

After `CRD` and `deployment/slime-boot` are successfully installed, users can manually apply the following yaml manifest to install the limiter module which supporting single and average [limiter](./install/limiter.yaml)

**global shared rate limit**, please read [install Rls & Redis](#install-rls), and install the limiter module [limiter-global](../install/limiter-global.yaml)

If you need to support adaptive flow limiting, please read [install Prometheus](#install-prometheus)

## SmartLimiter

For smarlimiter definition see [proto](https://raw.githubusercontent.com/slime-io/slime/master/staging/src/slime.io/slime/modules/limiter/api/v1alpha2/smart_limiter.proto)

**Note**: In the mesh, only one SmartLimiter resource can be created per service, whose name and namespace correspond to the name and namespace of the service's service

### single smartlimiter in mesh

- A->B, the request is rate limiting in B

The following smartlimiter indicates that the inbound direction of the reviews service is restricted to port 9080, and the restriction rule is 100 times/60 seconds

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

- A->B, the request is rate limiting in A

The following smartlimiter indicates that outbound direction of the reviewsis restricted, that is, and the restriction rule is 2 times/10 seconds

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

- smartlimiter with match in mesh

The following smartlimiter means that for the productpage service in default, port 9080

if the header of the request contains foo=bar, then the request is limited, and the restriction rule is 5 times/60s

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

in addition to support Header's exact match, but also support regular match, prefix match, suffix match, existence and other，refer to[SmartLimitDescriptor](../api/v1alpha2/smart_limiter.proto)


### global average smartlimiter in mesh

The following smartlimiter indicates that for port 9080 of the reviews service in default

the request is limited to a total of 100 times/60s, if review has two pods, the restriction rule is 50 times/60s per pod

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

### global share smartlimiter in mesh

All pods of the service, share a global counter

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

### single smartlimiter in gw

outbound is valid in gw

the following smartlimiter indicates: pods with lables `gw_cluster: prod-gateway` will apply restriction rules

request to route a.test.com:80/r1 is 1 time/60s

```yaml
apiVersion: microservice.slime.io/v1alpha2
kind: SmartLimiter
metadata:
  name: prod-gateway-1
  namespace: gateway-system
spec:
  gateway: true 
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60 
          quota: "1"     
          strategy: single
        condition: "true"
        target:
          direction: outbound
          route:
          - a.test.com:80/r1 
  workloadSelector:
    gw_cluster: prod-gateway  
```

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

### global share smartlimiter in gw


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

## Practices

### Practice 1: global average smartlimiter in mesh 

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

result:

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

envoyfilter is like:

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

### Practice 2: global share smartlimiter in mesh

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

and smarlimiter will covert like this:

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

result:

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

envoyfilter is like:

~~~yaml
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

configmap is like:
~~~yaml
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

## Dependencies

### install Prometheus

```
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/master/staging/src/slime.io/slime/modules/limiter/install/prometheus.yaml"
```

### install RLS

~~~shell
kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/master/staging/src/slime.io/slime/modules/limiter/install/rls.yaml"
~~~
