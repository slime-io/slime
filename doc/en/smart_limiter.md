- [Install & Use](#install--use)
- [Adaptive ratelimit based on metrics](#adaptive-ratelimit-based-on-metrics)
- [Subset RateLimit](#subset-ratelimit)
- [Service ratelimit](#service-ratelimit)
- [Example](#example)
  - [Install istio (1.8+)](#install-istio-18)
  - [Set Tag](#set-tag)
  - [Install slime](#install-slime)
  - [Install Bookinfo](#install-bookinfo)
  - [Create Smartlimiter](#create-smartlimiter)
  - [Observ Smartlimiter](#observ-smartlimiter)
  - [Observ Envoyfilter](#observ-envoyfilter)
  - [Visit and Observ](#visit-and-observ)
  - [Uninstall](#uninstall)
  - [Remarks](#remarks)

#### Install & Use

Make sure slime-boot has been installed.

Install the limiter module, through slime-boot:

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
    tag: {{your_limiter_tag}}
  module:
    - limiter:
        enable: true
        backend: 1
      metric:
        prometheus:
          address: {{prometheus_address}} # replace to your prometheus address
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
      name: limiter   
```

[Example](../../install/samples/smartlimiter/slimeboot_smartlimiter.yaml)

In the example, we configure prometheus as the monitoring source, and "prometheus handlers" defines the attributes that we want to obtain from monitoring. These attributes can be used as parameters in the traffic rules to achieve the purpose of adaptive ratelimit. 
Users can also define the monitoring attributes that the limiter module needs to obtain according to their needs. The following are some commonly used statements for obtaining monitoring attributes:

```
CPU:
Sum：
sum(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""})
Max：
max(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""})
Limit:
container_spec_cpu_quota{pod=~"$pod_name"}

Memory：
Sum：
sum(container_memory_usage_bytes{namespace="$namespace",pod=~"$pod_name",image=""})
Max：
max(container_memory_usage_bytes{namespace="$namespace",pod=~"$pod_name",image=""})
Limit:
sum(container_spec_memory_limit_bytes{pod=~"$pod_name"})

Request Duration：
90：
histogram_quantile(0.90, sum(rate(istio_request_duration_milliseconds_bucket{kubernetes_pod_name=~"$pod_name"}[2m]))by(le))
95：
histogram_quantile(0.95, sum(rate(istio_request_duration_milliseconds_bucket{kubernetes_pod_name=~"$pod_name"}[2m]))by(le))
99：
histogram_quantile(0.99, sum(rate(istio_request_duration_milliseconds_bucket{kubernetes_pod_name=~"$pod_name"}[2m]))by(le))
```



####  Adaptive ratelimit based on metrics

The metrics information entry can be configured in `condition`. The data from prometheus shows in metricStatus.

Deploy

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  sets:
    v2:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "1"
        condition: '{{.v2.cpu.sum}}>10'
```

Get

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: SmartLimiter
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"microservice.slime.io/v1alpha1","kind":"SmartLimiter","metadata":{"annotations":{},"name":"reviews","namespace":"default"},"spec":{"sets":{"v2":{"descriptor":[{"action":{"fill_interval":{"seconds":60},"quota":"1"},"condition":"{{.v2.cpu.sum}}\u003e100"}]}}}}
  creationTimestamp: "2021-08-09T07:54:24Z"
  generation: 1
  name: reviews
  namespace: default
  resourceVersion: "63006"
  uid: e6bf5121-6126-40db-8f5c-e124894b5d54
spec:
  sets:
    v2:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "1"
        condition: '{{.v2.cpu.sum}}>100'
status:
  metricStatus:
    _base.cpu.max: "181.591794594"
    _base.cpu.sum: "536.226646691"
    _base.pod: "3"
    v1.cpu.max: "176.285405855"
    v1.cpu.sum: "176.285405855"
    v1.pod: "1"
    v2.cpu.max: "178.349446242"
    v2.cpu.sum: "178.349446242"
    v2.pod: "1"
    v3.cpu.max: "181.591794594"
    v3.cpu.sum: "181.591794594"
    v3.pod: "1"
  ratelimitStatus:
    v2:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "1"
```

The formula in the condition will be rendered according to the entry of endPointStatus. If the result of the rendered formula is true, the limit will be triggered.



#### Subset RateLimit

In istio's system, users can define subsets for services through DestinationRule, and customize service traffic rules such as load balancing and connection pooling for them. RateLimit also belongs to this kind of service traffic rules. Through the slime framework, we can not only customize the rateLimit rules for services, but also for subsets, as shown below:

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  sets:
    v1: # reviews v1
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "1"
        condition: "true"
```

The above configuration limits 1 requests per minute for the v1 version of the reviews service. After submitting the configuration, the status information and ratelimit information of the instance under the service will be displayed in `status`, as follows:

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: SmartLimiter
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"microservice.slime.io/v1alpha1","kind":"SmartLimiter","metadata":{"annotations":{},"name":"reviews","namespace":"default"},"spec":{"sets":{"v1":{"descriptor":[{"action":{"fill_interval":{"seconds":60},"quota":"1"},"condition":"true"}]}}}}
  creationTimestamp: "2021-08-09T07:06:42Z"
  generation: 1
  name: reviews
  namespace: default
  resourceVersion: "59635"
  uid: ba62ff40-3c9f-427d-959b-6a416d54f24c
spec:
  sets:
    v1:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "1"
        condition: "true"
status:
  metricStatus:
    _base.cpu.max: "158.942939832"
    _base.cpu.sum: "469.688066909"
    _base.pod: "3"
    v1.cpu.max: "154.605786157"
    v1.cpu.sum: "154.605786157"
    v1.pod: "1"
    v2.cpu.max: "156.13934092"
    v2.cpu.sum: "156.13934092"
    v2.pod: "1"
    v3.cpu.max: "158.942939832"
    v3.cpu.sum: "158.942939832"
    v3.pod: "1"
  ratelimitStatus:
    v1:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "1"
```


#### Service ratelimit

Due to the lack of global quota management components, we cannot achieve precise service ratelimit, but assuming that the load balance is ideal, 
(instance quota) = (service quota)/(the number of instances). The service quota of test-svc is 3, then the quota field can be configured to 3/{pod} to achieve service-level ratelimit. When the service is expanded, you can see the change of the instance quota in status.

Deploy

```yaml
apiVersion: microservice.slime.io/v1alpha1
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
          quota: 3/{{._base.pod}}
        condition: "true"
```

Get

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: SmartLimiter
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"microservice.slime.io/v1alpha1","kind":"SmartLimiter","metadata":{"annotations":{},"name":"reviews","namespace":"default"},"spec":{"sets":{"_base":{"descriptor":[{"action":{"fill_interval":{"seconds":60},"quota":"3/{{._base.pod}}"},"condition":"true"}]}}}}
  creationTimestamp: "2021-08-09T08:21:11Z"
  generation: 1
  name: reviews
  namespace: default
  resourceVersion: "65036"
  uid: 16fc8c81-f71a-45ae-be1f-8f38d9a1fe4b
spec:
  sets:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: 3/{{._base.pod}}
        condition: "true"
status:
  metricStatus:
    _base.cpu.max: "192.360021503"
    _base.cpu.sum: "566.4194305139999"
    _base.pod: "3"
    v1.cpu.max: "185.760390031"
    v1.cpu.sum: "185.760390031"
    v1.pod: "1"
    v2.cpu.max: "188.29901898"
    v2.cpu.sum: "188.29901898"
    v2.pod: "1"
    v3.cpu.max: "192.360021503"
    v3.cpu.sum: "192.360021503"
    v3.pod: "1"
  ratelimitStatus:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "1" # For each instance, the current limit quota is 3/3=1.
```




#### Example

##### Install istio (1.8+)



##### Set Tag

$latest_tag equals the latest tag. The shell scripts and yaml files uses this version as default.

```sh
$ export latest_tag=$(curl -s https://api.github.com/repos/slime-io/slime/tags | grep 'name' | cut -d\" -f4 | head -1)
```



##### Install slime

```sh
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/samples/smartlimiter/easy_install_limiter.sh)"
```

Make sure all component are running.

```sh
$ kubectl get slimeboot -n mesh-operator
NAME           AGE
smartlimiter   6s
$ kubectl get pod -n mesh-operator
NAME                                    READY   STATUS    RESTARTS   AGE
limiter-6cb886d74-82hxg                 1/1     Running   0          26s
slime-boot-5977685db8-lnltl             1/1     Running   0          6m22s
```



##### Install Bookinfo

Change the namespace of current-context to which bookinfo will deploy first. Here we use default namespace.

```sh
$ kubectl label namespace default istio-injection=enabled
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/config/bookinfo.yaml"
```

Then we can visit productpage from pod/ratings, executing `curl productpage:9080/productpage`. 

You can also create gateway and visit productpage from outside, like what shows in  [Open the application to the outside](https://istio.io/latest/docs/setup/getting-started/#ip).

Create DestinationRule for reviews.

```sh
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/config/reviews-destination-rule.yaml"
```



##### Create Smartlimiter

```sh
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/samples/smartlimiter/smartlimiter_reviews.yaml"
```



##### Observ Smartlimiter

```sh
$ kubectl get smartlimiter reviews -oyaml
apiVersion: microservice.slime.io/v1alpha1
kind: SmartLimiter
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"microservice.slime.io/v1alpha1","kind":"SmartLimiter","metadata":{"annotations":{},"name":"reviews","namespace":"default"},"spec":{"sets":{"v2":{"descriptor":[{"action":{"fill_interval":{"seconds":60},"quota":"1"},"condition":"{{.v2.cpu.sum}}\u003e100"}]}}}}
  creationTimestamp: "2021-08-09T07:54:24Z"
  generation: 1
  name: reviews
  namespace: default
  resourceVersion: "63006"
  uid: e6bf5121-6126-40db-8f5c-e124894b5d54
spec:
  sets:
    v2:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "1"
        condition: '{{.v2.cpu.sum}}>100'
status:
  metricStatus:
    _base.cpu.max: "181.591794594"
    _base.cpu.sum: "536.226646691"
    _base.pod: "3"
    v1.cpu.max: "176.285405855"
    v1.cpu.sum: "176.285405855"
    v1.pod: "1"
    v2.cpu.max: "178.349446242"
    v2.cpu.sum: "178.349446242"
    v2.pod: "1"
    v3.cpu.max: "181.591794594"
    v3.cpu.sum: "181.591794594"
    v3.pod: "1"
  ratelimitStatus:
    v2:
      descriptor:
      - action:
          fill_interval:
            seconds: 60
          quota: "1"
```



##### Observ Envoyfilter

```sh
$ kubectl get envoyfilter reviews.default.v2.ratelimit -oyaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  creationTimestamp: "2021-08-09T07:54:25Z"
  generation: 1
  name: reviews.default.v2.ratelimit
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: SmartLimiter
    name: reviews
    uid: e6bf5121-6126-40db-8f5c-e124894b5d54
  resourceVersion: "62926"
  uid: cbea7344-3fda-4a1d-b3d2-93649ce76129
spec:
  configPatches:
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
          '@type': type.googleapis.com/udpa.type.v1.TypedStruct
          type_url: type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
          value:
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
                seconds: "60"
              max_tokens: 1
  workloadSelector:
    labels:
      app: reviews
      version: v2
```



##### Visit and Observ

Get accesslog of productpage

```
[2021-08-09T08:01:04.872Z] "GET /reviews/0 HTTP/1.1" 429 - via_upstream - "-" 0 18 1 1 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "19605264-5868-9c90-8b42-424790fad1b2" "reviews:9080" "172.17.0.11:9080" outbound|9080||reviews.default.svc.cluster.local 172.17.0.16:41342 10.100.112.212:9080 172.17.0.16:41378 - default
```

Get accesslog of review-v2

```
[2021-08-09T08:01:04.049Z] "GET /reviews/0 HTTP/1.1" 429 - local_rate_limited - "-" 0 18 0 - "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "3f0a65c8-1c66-9994-a0cf-1d7ae0446371" "reviews:9080" "-" inbound|9080|| - 172.17.0.11:9080 172.17.0.16:41342 outbound_.9080_._.reviews.default.svc.cluster.local -
```

Response code is 429. The smartlimiter is working.



##### Uninstall

Uninstall bookinfo.

```sh
$ kubectl delete -f "https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/config/bookinfo.yaml"
$ kubectl delete -f "https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/config/reviews-destination-rule.yaml"
```

Uninstall slime.

```sh
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/samples/smartlimiter/easy_uninstall_limiter.sh)"
```



##### Remarks

If you want to use customize shell scripts or yaml files, please set $custom_tag_or_commit. 

```sh
$ export custom_tag_or_commit=xxx
```

If command includes a yaml file,  please use $custom_tag_or_commit instead of $latest_tag.

```sh
#$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/config/bookinfo.yaml"
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$custom_tag_or_commit/install/config/bookinfo.yaml"
```

If command includes a shell script,  please add $custom_tag_or_commit as a parameter to the shell script.

```sh
#$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/samples/smartlimiter/easy_install_limiter.sh)"
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/samples/smartlimiter/easy_install_limiter.sh)" $custom_tag_or_commit
```

