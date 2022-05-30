- [Lazyload Turotails](#lazyload-turotails)
  - [Architecture](#architecture)
  - [Install-and-Use](#install-and-use)
    - [Cluster Mode](#cluster-mode)
      - [Accesslog](#accesslog)
      - [Prometheus](#prometheus)
    - [Namespace Mode](#namespace-mode)
      - [Accesslog](#accesslog-1)
      - [Prometheus](#prometheus-1)
  - [Introduction of features](#introduction-of-features)
    - [Enable lazyload based on accesslog](#enable-lazyload-based-on-accesslog)
    - [Automatic ServiceFence generation based on namespace/service label](#automatic-servicefence-generation-based-on-namespaceservice-label)
    - [Custom undefined traffic dispatch](#custom-undefined-traffic-dispatch)
    - [Support for adding static service dependencies](#support-for-adding-static-service-dependencies)
      - [Dependency on specific services](#dependency-on-specific-services)
      - [Dependency on all services in  specific namespaces](#dependency-on-all-services-in--specific-namespaces)
      - [Dependency on all services with specific labels](#dependency-on-all-services-with-specific-labels)
    - [Support for custom service dependency aliases](#Support for custom service dependency aliases)
    - [Logs output to local file and rotate](#logs-output-to-local-file-and-rotate)
      - [Creating Storage Volumes](#creating-storage-volumes)
      - [Declaring mount information in SlimeBoot](#declaring-mount-information-in-slimeboot)
  - [Example](#example)
    - [Install Istio (1.8+)](#install-istio-18)
    - [Set Tag](#set-tag)
    - [Install Slime](#install-slime)
    - [Install Bookinfo](#install-bookinfo)
    - [Enable Lazyload](#enable-lazyload)
    - [First Visit and Observ](#first-visit-and-observ)
    - [Second Visit and Observ](#second-visit-and-observ)
    - [Uninstall](#uninstall)
    - [Remarks](#remarks)
  - [FAQ](#faq)
    - [Istio versions supported?](#istio-versions-supported)
    - [Why is it necessary to specify a service port?](#why-is-it-necessary-to-specify-a-service-port)
    - [Why is it necessary to specify a list of namespaces for lazyload?](#why-is-it-necessary-to-specify-a-list-of-namespaces-for-lazyload)
    - [~~Meaning of global-sidecar-pilot?~~ (component obsolete)](#meaning-of-global-sidecar-pilot-component-obsolete)
    - [~~global sidecar does not start properly?~~ (Solved)](#global-sidecar-does-not-start-properly-solved)




# Lazyload Turotails

## Architecture



![](./media/lazyload-architecture-20211222.png)

* The green arrows are the internal logic of lazyload controller, and the orange arrows are the internal logic of global-sidecar. 

Instruction：

2. Deploy the Lazyload module, and Istiod will inject the standard sidecar (envoy) for the global-sidecar application

2. Enable lazyload for Service A

   2.1 Create ServiceFence A

   2.2 Create Sidecar(Istio CRD) A, and initialize accrording to the static config of ServiceFence.spec

   2.3 ApiServer senses sidecar A creation

3. Istiod gets the content of Sidecar A

4. Istiod pushed new configuration to sidecar of Service A

5. Service A sends first request to Service B. sidecar A does not has information about Service B, then request is sent to global-sidecar.

6. Global-sidecar operations

   6.1 Inbound traffic is intercepted, and in accesslog mode, sidecar generates an accesslog containing the service invocation relationship

   6.2 The global-sidecar application converts the access target to Service B based on the request header and other information

   6.3 Outbound traffic interception, where sidecar has all the service configuration information, finds the Service B target information and sends the request

7. Request sends to Service B

8. Global-sidecar reports relationships through access log or prometheus metric

9. Lazyload controller gets the relationships

10. lazyload controller updates lazyload configuration

    10.1 Update ServiceFence A adding infomation about Service B

    10.2 Update Sidecar A，adding egress.hosts of Service B

    10.3 ApiServer senses Sidecar A update

11. Istiod gets the content of Sidecar A

12. Istiod pushed new configuration to sidecar of Service A

13. Service A sends request to Service B directly



## Install-and-Use

To use lazyload module, add `fence` to `SlimeBoot.spec.module` and `enable: true`, and specify how the global-sidecar component is used, like follows

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  module:
    - name: lazyload # custom value
      kind: lazyload # should be "lazyload"
      enable: true
      general: # replace previous "fence" field
        # other config
  component:
    globalSidecar:
      enable: true
      # other config
```



Depending on how the global-sidecar is deployed and the source of the metrics on which the service depends, there are four modes.



### Cluster Mode

In this mode, all namespaces in the service mesh can use Lazyload, no need to explicitly specify a list of namespace like the namespace mode. This mode deploys a global-sidecar application, in the namespace of the lazyload controller, defaulting to mesh-operator. 

#### Accesslog

The source of the metrics is the global-sidecar's accesslog.

> [Full Example](./install/samples/lazyload/slimeboot_cluster_accesslog.yaml)

```yaml
---
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-lazyload
    tag: {{your_lazyload_tag}}
  module:
    - name: lazyload # custom value
      kind: lazyload # should be "lazyload"
      enable: true
      general: # replace previous "fence" field
        wormholePort: # replace to your application service ports, and extend the list in case of multi ports
          - "{{your_port}}"
      global:
        misc:
          globalSidecarMode: cluster # inform the lazyload controller of the global-sidecar mode
          metricSourceType: accesslog # infrom the metric source
  component:
    globalSidecar:
      enable: true
      type: cluster # inform the slime-boot operator of the global-sidecar mode
      sidecarInject:
        enable: true # must be true
        mode: pod # if type = cluster, can only be "pod"; if type = namespace, can be "pod" or "namespace"
        labels: # optional, used for sidecarInject.mode = pod, indicate the labels for sidecar auto inject 
          {{your_istio_autoinject_labels}}
      resources:
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 400m
          memory: 400Mi
      image:
        repository: docker.io/slimeio/slime-global-sidecar
        tag: {{your_global-sidecar_tag}}
```



#### Prometheus

The source of the metrics is Prometheus.

> [Full Example](./install/samples/lazyload/slimeboot_cluster_prometheus.yaml)

```yaml
---
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-lazyload
    tag: {{your_lazyload_tag}}
  module:
    - name: lazyload # custom value
      kind: lazyload # should be "lazyload"
      enable: true
      general: # replace previous "fence" field
        wormholePort: # replace to your application service ports, and extend the list in case of multi ports
          - "{{your_port}}"
      global:
        misc:
          globalSidecarMode: cluster # inform the lazyload controller of the global-sidecar mode
      metric: # indicate the metric source
        prometheus:
          address: {{your_prometheus_address}}
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",reporter="destination"})by(destination_service)
              type: Group
  component:
    globalSidecar:
      enable: true
      type: cluster # inform the slime-boot operator of the global-sidecar mode
      sidecarInject:
        enable: true # must be true
        mode: pod # if type = cluster, can only be "pod"; if type = namespace, can be "pod" or "namespace"
        labels: # optional, used for sidecarInject.mode = pod, indicate the labels for sidecar auto inject 
          {{your_istio_autoinject_labels}}
      resources:
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 400m
          memory: 400Mi
      image:
        repository: docker.io/slimeio/slime-global-sidecar
        tag: {{your_global-sidecar_tag}}
```



### Namespace Mode

This pattern deploys a global-sidecar application in each namespace where lazyload is intended to be used. Underwriting requests for each namespace are sent to the global-sidecar application under the same namespace. 

#### Accesslog

The source of the metrics is the global-sidecar's accesslog.

> [Full Example](./install/samples/lazyload/slimeboot_namespace_accesslog.yaml)

```yaml
---
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-lazyload
    tag: {{your_lazyload_tag}}
  module:
    - name: lazyload # custom value
      kind: lazyload # should be "lazyload"
      enable: true
      general: # replace previous "fence" field
        wormholePort: # replace to your application service ports, and extend the list in case of multi ports
          - "{{your_port}}"
        namespace: # replace to your service's namespace which will use lazyload, and extend the list in case of multi namespaces
          - {{your_namespace}}
      global:
        misc:
          metricSourceType: accesslog # indicate the metric source
  component:
    globalSidecar:
      enable: true
      type: namespace # inform the slime-boot operator of the global-sidecar mode
      sidecarInject:
        enable: true # must be true
        mode: namespace # if type = cluster, can only be "pod"; if type = namespace, can be "pod" or "namespace"
        #labels: # optional, used for sidecarInject.mode = pod
          #sidecar.istio.io/inject: "true"
      resources:
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 400m
          memory: 400Mi
      image:
        repository: docker.io/slimeio/slime-global-sidecar
        tag: {{your_global-sidecar_tag}}
```



#### Prometheus

The source of the metrics is Prometheus.

>[Full Example](./install/samples/lazyload/slimeboot_namespace_prometheus.yaml)

```yaml
---
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-lazyload
    tag: {{your_lazyload_tag}}
  module:
    - name: lazyload # custom value
      kind: lazyload # should be "lazyload"
      enable: true
      general: # replace previous "fence" field
        wormholePort: # replace to your application service ports, and extend the list in case of multi ports
          - "{{your_port}}"
        namespace: # replace to your service's namespace which will use lazyload, and extend the list in case of multi namespaces
          - {{your_namespace}}
      metric: # indicate the metric source
        prometheus:
          address: http://prometheus.istio-system:9090
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",reporter="destination"})by(destination_service)
              type: Group
  component:
    globalSidecar:
      enable: true
      type: namespace # inform the slime-boot operator of the global-sidecar mode
      sidecarInject:
        enable: true # must be true
        mode: namespace # if type = cluster, can only be "pod"; if type = namespace, can be "pod" or "namespace"
        #labels: # optional, used for sidecarInject.mode = pod
          #sidecar.istio.io/inject: "true"
      resources:
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 400m
          memory: 400Mi
      image:
        repository: docker.io/slimeio/slime-global-sidecar
        tag: {{your_global-sidecar_tag}}
```







## Introduction of features

### Enable lazyload based on accesslog

Specifying the SlimeBoot CR resource `spec.module.global.misc.metricSourceType` equal to `accesslog` will use Accesslog to get the service  relationship, and equal to `prometheus` will use Prometheus.

Approximate process of obtaining service call relationships using Accesslog:

- When slime-boot creates global-sidecar, it finds `metricSourceType: accesslog` and generates an additional configmap with static_resources containing the address information for the lazyload controller to process accesslog. The static_resources is then added to the global-sidecar configuration by an envoyfilter, so that the global-sidecar accesslog will be sent to the lazyload controller
- The global-sidecar generates an accesslog, containing information about the caller and callee services. Global-sidecar sends the information to the lazyload controller
- The lazyload controller analyzes the accesslog and gets the new service call relationship

The subsequent process, which involves modifying servicefence and sidecar, is the same as the process for handling the prometheus metric.

Example

```yaml
spec:
  module:
    - name: lazyload # custom value
      kind: lazyload # should be "lazyload"
      enable: true
      general: # replace previous "fence" field
        wormholePort: # replace to your application svc ports
          - "9080"
      global:
        misc:
          metricSourceType: accesslog
```

[Full sample](./install/samples/lazyload/slimeboot_cluster_accesslog.yaml)



### Support for enabling lazyload for services manually or automatically

Support for enabling lazyload for services, either manually or automatically, via the `autoFence` parameter. Enabling lazyload here refers to the creation of the serviceFence resource, which generates the Sidecar CR.

Support for specifying whether lazyload is globally enabled in automatic mode via the `defaultFence` parameter.

The configuration is as follows

```yaml
---
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  module:
    - name: lazyload
      kind: lazyload
      enable: true
      general:
        autoFence: true # true for automatic mode, false for manual mode, default for manual mode
        defaultFence: true # Default behaviour in auto mode, true to create servicefence, false to not create, default not to create
  # ...
```



#### Auto mode

Auto mode is entered when the `autoFence` parameter is `true`. The range of services enabled for lazyload in auto mode is adjusted by three dimensions.

Service Level - label `slime.io/serviceFenced`

* `false`: not auto enable
* `true`: auto enable

* other values or empty: use namespace level configuration

Namespace Level - label `slime.io/serviceFenced`

* `false`: not auto enable for this namespace
* `true`: auto enable for this namespace
* other values or empty: use global level configuration

Global Level - `defaultFence` param of lazyload module

- `false`: not auto enable for all 
- `true`: auto enable for all 

Priority: Service Level > Namespace Level > Global Level



Note: ServiceFence that are auto generated are labeled with `app.kubernetes.io/created-by=fence-controller`, which enables state association changes. ServiceFence that do not match this Label are considered to be manually configured and are not affected by the above Label.



**Example**

> Namespace `testns` has 3 services, `svc1`, `svc2`, `svc3`

* When `autoFence` is `true` and `defaultFence` is `true`, three ServiceFence for the above services is auto generated
* Label ns with `slime.io/serviceFenced: "false"`, all ServiceFence disappear
* Label `svc1` with `slime.io/serviceFenced: "true"` ,  create ServiceFence for `svc1`
* Delete the labels on Namespace and Service: created the three ServiceFence



**Sample configuration**

```yaml
apiVersion: v1
kind: Namespace
metadata:
  labels:
    istio-injection: enabled
    slime.io/serviceFenced: "false"
  name: testns
---
apiVersion: v1
kind: Service
metadata:
  annotations: {}
  labels:
    app: svc1
    service: svc1
    slime.io/serviceFenced: "true"
  name: svc1
  namespace: testns
```



#### Manual mode

When the `autoFence` parameter is `false`, lazyload is enabled in manual mode, requiring the user to create the ServiceFence resource manually. This enablement is Service level.



### Custom undefined traffic dispatch

By default, lazyload/fence sends  (default or undefined) traffic that envoy cannot match the route to the global sidecar to deal with the problem of missing service data temprorarily, which is inevitably faced by "lazy loading". This solution is limited by technical details, and cannot handle traffic whose target (e.g. domain name) is outside the cluster, see [[Configuration Lazy Loading]: Failed to access external service #3](https://github.com/slime-io/slime/issues/3).

Based on this background, this feature was designed to be used in more flexible business scenarios as well. The general idea is to assign different default traffic to different targets for correct processing by means of domain matching.



Sample configuration.

```yaml
module:
  - name: lazyload
    kind: lazyload
    enable: true
    general:
      wormholePort:
      - "80"
      - "8080"
      dispatches: # new field
      - name: 163
        domains:
        - "www.163.com"
        cluster: "outbound|80||egress1.testns.svc.cluster.local" # standard istio cluster format: <direction>|<svcPort>|<subset>|<svcFullName>, normally direction is outbound and subset is empty      
      - name: baidu
        domains:
        - "*.baidu.com"
        - "baidu.*"
        cluster: "{{ (print .Values.foo \ ". \" .Values.namespace ) }}" # you can use template to construct cluster dynamically
      - name: sohu
        domains:
        - "*.sohu.com"
        - "sodu.*"
        cluster: "_GLOBAL_SIDECAR" # a special name which will be replaced with actual global sidecar cluster
      - name: default
        domains:
        - "*"
        cluster: "PassthroughCluster"  # a special istio cluster which will passthrough the traffic according to orgDest info. It's the default behavior of native istio.
```

> In this example, we dispatch a portion of the traffic to the specified cluster; let another part go to the global sidecar; and then for the rest of the traffic, let it keep the native istio behavior: passthrough.



**Note**:

* In custom assignment scenarios, if you want to keep the original logic "all other undefined traffic goes to global sidecar", you need to explicitly configure the second from bottom item as above.



### Support for adding static service dependencies

In addition to updating service dependencies from the slime metric based on dynamic metrics, Lazyload also supports adding static service dependencies via `serviceFence.spec`. Three breakdown scenarios are supported: dependency on specific services, dependency on all services in  specific namespaces, dependency on all services with specific labels.

It is worth noting that static service dependencies, like dynamic service dependencies, also support the determination of the real backend of a service based on VirtualService rules and automatically extend the scope of Fence. Details at [ServiceFence Instruction](./README.md#ServiceFence-Instruction)





#### Dependency on specific services

For scenarios where a lazyload enabled service statically depends on one or more other services, the configuration can be added directly to the sidecar crd at initialization time.

In the following example, a static dependency on the `reviews.default` service is added for a lazyload enabled service.

```yaml
# servicefence
spec:
  enable: true
  host:
    reviews.default.svc.cluster.local: # static dependenct of reviews.default service
      stable:

# related sidecar
spec:
  egress:
  - hosts:
    - '*/reviews.default.svc.cluster.local'
    - istio-system/*
    - mesh-operator/*
```



#### Dependency on all services in  specific namespaces

For scenarios where a lazyload enabled service statically depends on all services in one or more other namespaces, the configuration can be added directly to the sidecar crd at initialization time.

In the following example, a static dependency on all services in the `test` namespace is added for a lazyload enabled service.

```yaml
# servicefence
spec:
  enable: true
  host:
    test/*: {} # static dependency of all services in test namespace

# related sidecar
spec:
  egress:
  - hosts:
    - test/*
    - istio-system/*
    - mesh-operator/*
```



#### Dependency on all services with specific labels

For scenarios where a lazyload enabled service has static dependencies on all services with a label or multiple labels, the configuration can be added directly to the sidecar crd at initialization time.

In the example below, static dependencies are added for all services with `app=details` and for all services with `app=reviews, group=default` for the lazyloading enabled service.

```yaml
# servicefence
spec:
  enable: true
  labelSelector: # Match service label, multiple selectors are 'or' relationship
    - selector:
        app: details
    - selector: # labels in one selector are 'and' relationship
        app: reviews
        group: default

# related sidecar
spec:
  egress:
  - hosts:
    - '*/details.default.svc.cluster.local' # with label "app=details"
    - '*/details.test.svc.cluster.local' # with label "app=details"
    - '*/reviews.default.svc.cluster.local' # with label "app=details" and "group=default"
    - istio-system/*
    - mesh-operator/*
```





### Support for custom service dependency aliases

In some scenarios, we want Lazyload to add some additional dependent services in based on the known dependent service.

Users can configure `general.domainAliases` to provide custom conversion relationships to achieve the requirements. `general.domainAliases` consists of one or many `domainAlias`. `domainAlias` consists of a matching rule `pattern` and a transformation rule `templates`.  `pattern` contains only one matching rule, while `templates` can contain multiple conversion rules.

For example, we want to add `<svc>. <ns>.svc.cluster.local` with an additional `<ns>. <svc>.mailsaas` service dependency, you can configure it like this

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  module:
    - name: lazyload-test
      kind: lazyload
      enable: true
      general:
        wormholePort: # replace to your application service ports, and extend the list in case of multi ports
          - "9080"
        domainAliases: 
          - pattern: '(?P<service>[^\.]+)\.(?P<namespace>[^\.]+)\.svc\.cluster\.local$'
            templates:
              - "$namespace.$service.mailsaas"
  #...
```

Servicefence will look like this

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: ServiceFence
metadata:
  name: ratings
  namespace: default
spec:
  enable: true
  host:
    details.default.svc.cluster.local: # static dependent service
      stable: {}
status:
  domains:
    default.details.mailsaas: # static dependent service converted result
      hosts:
      - default.details.mailsaas
    default.productpage.mailsaas: # dynamic dependent service converted result
      hosts:
      - default.productpage.mailsaas
    details.default.svc.cluster.local:
      hosts:
      - details.default.svc.cluster.local
    productpage.default.svc.cluster.local:
      hosts:
      - productpage.default.svc.cluster.local
  metricStatus:
    '{destination_service="productpage.default.svc.cluster.local"}': "1" # dynamic dependent service
```

Sidecar will look like this

```yaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  name: ratings
  namespace: default
spec:
  egress:
  - hosts:
    - '*/default.details.mailsaas' # static dependent service converted result
    - '*/default.productpage.mailsaas' # dynamic dependent service converted result
    - '*/details.default.svc.cluster.local'
    - '*/productpage.default.svc.cluster.local'
    - istio-system/*
    - mesh-operator/*
  workloadSelector:
    labels:
      app: ratings
```





### Logs output to local file and rotate

slime logs are output to stdout by default, specifying `spec.module.global.log.logRotate` equal to `true` in the SlimeBoot CR resource will output the logs locally and start the log rotation, no longer to standard output.

The rotation configuration is also adjustable, and the default configuration is as follows, which can be overridden by displaying the individual values in the specified logRotateConfig.

```yaml
spec:
  module:
    - name: lazyload
      enable: true
      fence:
        wormholePort: # replace to your application svc ports
          - "9080"
      global:
        log:
          logRotate: true
          logRotateConfig:
            filePath: "/tmp/log/slime.log"
            maxSizeMB: 100
            maxBackups: 10
            maxAgeDay: 10
            compress: true
```

It is usually required to be used with storage volumes. After the storage volume is prepared, specify `spec.volumes` and `spec.containers.slime.volumeMounts` in the SlimeBoot CR resource to show the path where the storage volume will be mounted to the log local file.

Here is the full demo based on the minikube kubernetes scenario.

#### Creating Storage Volumes

Create a hostpath type storage volume based on the /mnt/data

```yaml
# hostPath pv for minikube demo
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: lazyload-claim
  namespace: mesh-operator
spec:
  storageClassName: manual
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 3Gi
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: lazyload-volumn
  labels:
    type: local
spec:
  storageClassName: manual
  capacity:
    storage: 5Gi
  accessModes:
    - ReadWriteOnce
  hostPath:
    path: "/mnt/data"
```

#### Declaring mount information in SlimeBoot

Specify in the SlimeBoot CR resource that the storage volume will be mounted to the "/tmp/log" path of the pod, so that the slime logs will be persisted to the /mnt/data path and will be automatically rotated.

```yaml
---
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-lazyload
    tag: master-e5f2d83-dirty_1b68486
  module:
    - name: lazyload # custom value
      kind: lazyload # should be "lazyload"
      enable: true
      general: # replace previous "fence" field
        wormholePort: # replace to your application svc ports
          - "9080"
      global:
        log:
          logRotate: true
          logRotateConfig:
            filePath: "/tmp/log/slime.log"
            maxSizeMB: 100
            maxBackups: 10
            maxAgeDay: 10
            compress: true
#...
  volumes:
    - name: lazyload-storage
      persistentVolumeClaim:
        claimName: lazyload-claim
  containers:
    slime:
      volumeMounts:
        - mountPath: "/tmp/log"
          name: lazyload-storage
```

[Full Example](./install/samples/lazyload/slimeboot_logrotate.yaml)



## Example

### Install Istio (1.8+)



### Set Tag

$latest_tag equals the latest tag. The shell scripts and yaml files uses this version as default.

```sh
$ export latest_tag=$(curl -s https://api.github.com/repos/slime-io/lazyload/tags | grep 'name' | cut -d\" -f4 | head -1)
```



### Install Slime

```sh
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/lazyload/$latest_tag/install/samples/lazyload/easy_install_lazyload.sh)"
```

Confirm all components are running.

```sh
$ kubectl get slimeboot -n mesh-operator
NAME       AGE
lazyload   12s
$ kubectl get pod -n mesh-operator
NAME                              READY   STATUS    RESTARTS   AGE
global-sidecar-7dd48b65c8-gc7g4   2/2     Running   0          18s
lazyload-85987bbd4b-djshs         1/1     Running   0          18s
slime-boot-6f778b75cd-4v675       1/1     Running   0          26s
```



### Install Bookinfo

Change the namespace of current-context to which bookinfo will deploy first. Here we use default namespace.

```sh
$ kubectl label namespace default istio-injection=enabled
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/lazyload/$latest_tag/install/config/bookinfo.yaml"
```

Confirm all pods are running.

```sh
$ kubectl get po -n default
NAME                              READY   STATUS    RESTARTS   AGE
details-v1-79f774bdb9-6vzj6       2/2     Running   0          60s
productpage-v1-6b746f74dc-vkfr7   2/2     Running   0          59s
ratings-v1-b6994bb9-klg48         2/2     Running   0          59s
reviews-v1-545db77b95-z5ql9       2/2     Running   0          59s
reviews-v2-7bf8c9648f-xcvd6       2/2     Running   0          60s
reviews-v3-84779c7bbc-gb52x       2/2     Running   0          60s
```

Then we can visit productpage from pod/ratings, executing `curl productpage:9080/productpage`.

You can also create gateway and visit productpage from outside, like what shows in  [Open the application to outside traffic](https://istio.io/latest/docs/setup/getting-started/#ip).



### Enable Lazyload

Create ServiceFence for productpage. Two ways:

- Create ServiceFence manually

```sh
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/lazyload/$latest_tag/install/samples/lazyload/servicefence_productpage.yaml"
```

- Update service to automatically create ServiceFence

```sh
$ kubectl label service productpage -n default slime.io/serviceFenced=true
```



Confirm servicefence and sidecar already exist.

```sh
$ kubectl get servicefence -n default
NAME          AGE
productpage   12s
$ kubectl get sidecar -n default
NAME          AGE
productpage   22s
$ kubectl get servicefence productpage -n default -oyaml
apiVersion: microservice.slime.io/v1alpha1
kind: ServiceFence
metadata:
  creationTimestamp: "2021-12-23T06:21:14Z"
  generation: 1
  labels:
    app.kubernetes.io/created-by: fence-controller
  name: productpage
  namespace: default
  resourceVersion: "10662886"
  uid: f21e7d2b-4ab3-4de0-9b3d-131b6143d9db
spec:
  enable: true
status: {}
$ kubectl get sidecar productpage -n default -oyaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  creationTimestamp: "2021-12-23T06:21:14Z"
  generation: 1
  name: productpage
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ServiceFence
    name: productpage
    uid: f21e7d2b-4ab3-4de0-9b3d-131b6143d9db
  resourceVersion: "10662887"
  uid: 85f9dc11-6d83-4b84-8d1b-14bd031cc57b
spec:
  egress:
  - hosts:
    - istio-system/*
    - mesh-operator/*
  workloadSelector:
    labels:
      app: productpage
```



### First Visit and Observ

Visit the productpage website, and use `kubectl logs -f productpage-xxx -c istio-proxy -n default` to observe the access log of productpage.

```
[2021-12-23T06:24:55.527Z] "GET /details/0 HTTP/1.1" 200 - via_upstream - "-" 0 178 12 12 "-" "curl/7.52.1" "7ec25152-ca8e-923b-a736-49838ce316f4" "details:9080" "172.17.0.10:80" outbound|9080||global-sidecar.mesh-operator.svc.cluster.local 172.17.0.11:45194 10.102.66.227:9080 172.17.0.11:40210 - -

[2021-12-23T06:24:55.552Z] "GET /reviews/0 HTTP/1.1" 200 - via_upstream - "-" 0 295 30 29 "-" "curl/7.52.1" "7ec25152-ca8e-923b-a736-49838ce316f4" "reviews:9080" "172.17.0.10:80" outbound|9080||global-sidecar.mesh-operator.svc.cluster.local 172.17.0.11:45202 10.104.97.115:9080 172.17.0.11:40880 - -

[2021-12-23T06:24:55.490Z] "GET /productpage HTTP/1.1" 200 - via_upstream - "-" 0 4183 93 93 "-" "curl/7.52.1" "7ec25152-ca8e-923b-a736-49838ce316f4" "productpage:9080" "172.17.0.11:9080" inbound|9080|| 127.0.0.6:48621 172.17.0.11:9080 172.17.0.7:41458 outbound_.9080_._.productpage.default.svc.cluster.local default
```

It is clearly that the banckend of productpage is global-sidecar.mesh-operator.svc.cluster.local.



Observe servicefence productpage.

```sh
$ kubectl get servicefence productpage -n default -oyaml
apiVersion: microservice.slime.io/v1alpha1
kind: ServiceFence
metadata:
  creationTimestamp: "2021-12-23T06:21:14Z"
  generation: 1
  labels:
    app.kubernetes.io/created-by: fence-controller
  name: productpage
  namespace: default
  resourceVersion: "10663136"
  uid: f21e7d2b-4ab3-4de0-9b3d-131b6143d9db
spec:
  enable: true
status:
  domains:
    details.default.svc.cluster.local:
      hosts:
      - details.default.svc.cluster.local
    reviews.default.svc.cluster.local:
      hosts:
      - reviews.default.svc.cluster.local
  metricStatus:
    '{destination_service="details.default.svc.cluster.local"}': "1"
    '{destination_service="reviews.default.svc.cluster.local"}': "1"
```



Observe sidecar productpage.

```YAML
$ kubectl get sidecar productpage -n default -oyaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  creationTimestamp: "2021-12-23T06:21:14Z"
  generation: 2
  name: productpage
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ServiceFence
    name: productpage
    uid: f21e7d2b-4ab3-4de0-9b3d-131b6143d9db
  resourceVersion: "10663141"
  uid: 85f9dc11-6d83-4b84-8d1b-14bd031cc57b
spec:
  egress:
  - hosts:
    - '*/details.default.svc.cluster.local'
    - '*/reviews.default.svc.cluster.local'
    - istio-system/*
    - mesh-operator/*
  workloadSelector:
    labels:
      app: productpage
```

Details and reviews are already added into sidecar!



### Second Visit and Observ

Visit the productpage website again, and use `kubectl logs -f productpage-xxx -c istio-proxy -n default` to observe the access log of productpage.

```
[2021-12-23T06:26:47.700Z] "GET /details/0 HTTP/1.1" 200 - via_upstream - "-" 0 178 13 12 "-" "curl/7.52.1" "899e918c-e44c-9dc2-9629-d8db191af972" "details:9080" "172.17.0.13:9080" outbound|9080||details.default.svc.cluster.local 172.17.0.11:50020 10.102.66.227:9080 172.17.0.11:42180 - default

[2021-12-23T06:26:47.718Z] "GET /reviews/0 HTTP/1.1" 200 - via_upstream - "-" 0 375 78 77 "-" "curl/7.52.1" "899e918c-e44c-9dc2-9629-d8db191af972" "reviews:9080" "172.17.0.16:9080" outbound|9080||reviews.default.svc.cluster.local 172.17.0.11:58986 10.104.97.115:9080 172.17.0.11:42846 - default

[2021-12-23T06:26:47.690Z] "GET /productpage HTTP/1.1" 200 - via_upstream - "-" 0 5179 122 121 "-" "curl/7.52.1" "899e918c-e44c-9dc2-9629-d8db191af972" "productpage:9080" "172.17.0.11:9080" inbound|9080|| 127.0.0.6:51799 172.17.0.11:9080 172.17.0.7:41458 outbound_.9080_._.productpage.default.svc.cluster.local default
```

The backends are details and reviews now.



### Uninstall

Uninstall bookinfo.

```sh
$ kubectl delete -f "https://raw.githubusercontent.com/slime-io/lazyload/$latest_tag/install/config/bookinfo.yaml"
```

Uninstall slime.

```sh
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/lazyload/$latest_tag/install/samples/lazyload/easy_uninstall_lazyload.sh)"
```



### Remarks

If you want to use customize shell scripts or yaml files, please set $custom_tag_or_commit.

```sh
$ export custom_tag_or_commit=xxx
```

If command includes a yaml file,  please use $custom_tag_or_commit instead of $latest_tag.

```sh
#$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/lazyload/$latest_tag/install/config/bookinfo.yaml"
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/lazyload/$custom_tag_or_commit/install/config/bookinfo.yaml"
```

If command includes a shell script,  please add $custom_tag_or_commit as a parameter to the shell script.

```sh
#$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/lazyload/$latest_tag/install/samples/smartlimiter/easy_install_limiter.sh)"
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/lazyload/$latest_tag/install/samples/smartlimiter/easy_install_limiter.sh)" $custom_tag_or_commit
```





## FAQ

### Istio versions supported?

Istio 1.8 onwards is supported, see [A note on the incompatibility of lazyload with some versions of istio](https://github.com/slime-io/slime/issues/145) for a detailed compatibility note.



### Why is it necessary to specify a service port? (Deprecated soon)

No service port information is specified, as usually when lazyload is enabled, the default content of the sidecar only contains service information for istio-system and mesh-operator, and some specific ports usually have no services exposed. Due to Istio's mechanism, only listener's that are meaninglessly routed in the pocket will be removed, and there is no guarantee that a placeholder listener will exist.

For example, if the service in bookinfo is exposed on port `9080`, and lazyload is enabled for the productpage service, the productpage `9080` listener will be removed. When you access `details:9080` after that, without the listener, you will go directly to the Passthrough logic and will not be able to go to the global-sidecar, and you will not be able to fetch the service dependencies.



### Why is it necessary to specify a list of namespaces for lazyload? (Deprecated soon)

Due to the large number of short domain access scenarios, different namespace information needs to be replenished in different namespaces. So lazyload will create separate envoyfilters under these specified namespaces, supplemented with the appropriate namespace information.



### ~~Meaning of global-sidecar-pilot?~~ (component obsolete)

~~Because the role of global-sidecar is different from normal sidecar, it needs some custom logic, such as the bottom envoyfilter does not take effect for global-sidecar or it will dead-loop, etc. global-sidecar does not directly use the full configuration of the original pilot of the cluster. The global-sidecar-pilot will get the full configuration from the original pilot of the cluster, then fine-tune it and push it to the global-sidecar. the existing global-sidecar-pilot is based on istiod 1.7.~~

~~Note: In order to reduce learning costs and enhance compatibility, we are considering removing the global-sidecar-pilot, when there will no longer be a customized pilot, fully compatible with the community version, and expect to implement this feature in the next major release.~~



### ~~global sidecar does not start properly?~~ (Solved)

~~Global sidecar starts with an error `Internal:Error adding/updating listener(s) 0.0.0.0_15021: cannot bind '0.0.0.0:15021': Address already in use`.~~ 

~~This error is usually caused by port conflict. The global-sidecar is a sidecar running in gateway mode, which binds to the real port. Specifically, istio-ingressgateway is using port 15021, which will cause the lds update of global-sidecar to fail, and can be solved by changing port 15021 of ingressgateway to another value.~~

~~Note: This problem is currently solved by port planning, and will be freed from this limitation in the next major release.~~

