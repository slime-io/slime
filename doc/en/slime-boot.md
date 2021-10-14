- [How to Use Slime](#how-to-use-slime)
  - [Install Slime-boot](#install-slime-boot)
  - [Install Prometheus](#install-prometheus)
  - [Verify](#verify)
- [slimeboot Default Value Instruction and Replacement Method](#slimeboot-default-value-instruction-and-replacement-method)
  - [Default Value Instruction](#default-value-instruction)
    - [values.yaml](#valuesyaml)
    - [Config.global](#configglobal)
  - [Replacement Method](#replacement-method)
    - [Example](#example)

## How to Use Slime

### Install Slime-boot

You can easily install and uninstall the slime sub-module with slime-boot. 

$tag_or_commit uses latest tag as default now. You can use other tag or commit_id instead as needed. Using the following commands to install slime-boot:

```sh
$ export tag_or_commit=$(curl -s https://api.github.com/repos/slime-io/slime/tags | grep 'name' | cut -d\" -f4 | head -1)
$ kubectl create ns mesh-operator
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/init/crds.yaml"
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/init/deployment_slime-boot.yaml"
```



### Install Prometheus

The lazy load and smart limiter module needs metric data, so we suggest you installing prometheus in your system. Here is a simple prometheus installation file copied from istio.io.

```sh
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/config/prometheus.yaml"
```



### Verify

After installation of slime-boot, check whether slime-boot pod in mesh-operator and prometheus pod in istio-system are running. 

```sh
$ kubectl get po -n mesh-operator
NAME                         READY   STATUS    RESTARTS   AGE
slime-boot-fd9d7ff6d-4qb5f   1/1     Running   0          3h25m
$ kubectl get po -n istio-system
NAME                                    READY   STATUS    RESTARTS   AGE
istio-egressgateway-78cb6c4799-6w2cn    1/1     Running   5          14d
istio-ingressgateway-59644976b5-kmw9s   1/1     Running   5          14d
istiod-664799f4bc-wvdhv                 1/1     Running   5          14d
prometheus-69f7f4d689-hrtg5             2/2     Running   2          4d4h
```





## slimeboot Default Value Instruction and Replacement Method

### Default Value Instruction

The default value source consists of two parts

1. values.yaml of slime-boot operator. The file path is "slime/slime-boot/helm-charts/slimeboot/values.yaml"
2. struct Config.Global of slime framework. The code path is "slime/slime-framework/apis/config/v1alpha1/config.proto"

#### values.yaml

Used by slimeboot operator templates to create slime module and slime component.

| Key                                        | Default Value | Usages                                                      | Remark                                                       |
| ------------------------------------------ | ------------- | ----------------------------------------------------------- | ------------------------------------------------------------ |
| replicaCount                               | 1             | module                                                      |                                                              |
| image.pullPolicy                           | Always        | module                                                      |                                                              |
| serviceAccount.create                      | true          | module                                                      | switch on serviceAccount creating                            |
| serviceAccount.annotations                 | { }           | -                                                           |                                                              |
| serviceAccount.name                        | ""            | -                                                           |                                                              |
| podAnnotations                             | { }           | -                                                           |                                                              |
| podSecurityContext                         | { }           | module                                                      |                                                              |
| containerSecurityContext                   | { }           | module                                                      |                                                              |
| service.type                               | ClusterIP     | module                                                      |                                                              |
| service.port                               | 80            | module                                                      |                                                              |
| resources.limits.cpu                       | 1             | module and component                                        |                                                              |
| resources.limits.memory                    | 1Gi           | module and component                                        |                                                              |
| resources.requests.cpu                     | 200m          | module and component                                        |                                                              |
| resources.requests.memory                  | 200Mi         | module and component                                        |                                                              |
| autoscaling.enabled                        | false         | -                                                           |                                                              |
| autoscaling.minReplicas                    | 1             | -                                                           |                                                              |
| autoscaling.maxReplicas                    | 100           | -                                                           |                                                              |
| autoscaling.targetCPUUtilizationPercentage | 80            | -                                                           |                                                              |
| nodeSelector                               | { }           | module                                                      |                                                              |
| tolerations                                | [ ]           | module                                                      |                                                              |
| affinity                                   | { }           | module                                                      |                                                              |
| namespace                                  | mesh-operator | module and component(cluster global-sidecar, pilot)         | namespace deployed slime                                     |
| istioNamespace                             | istio-system  | component(cluster global-sidecar, namespace global-sidecar) | namespace deployed istio                                     |
| healthProbePort                            | 8081          | module                                                      | If change, need to be the same with the port contained by config.global.misc["aux-addr"] |

#### Config.global

Only used by slime module.

| Key            | Default Value                                                | Usages                                                       | Remark |
| -------------- | ------------------------------------------------------------ | ------------------------------------------------------------ | ------ |
| Service        | app                                                          | label key selected by servicefence, for generating sidecar default configuration in lazyload |        |
| IstioNamespace | istio-system                                                 | namespace deployed istio, for generating sidecar default configuration in lazyload, should equal the real namespace deployed istio |        |
| SlimeNamespace | mesh-operator                                                | namespace deployed slime, for generating sidecar default configuration in lazyload, should equal the real namespace deployed slimeboot custom resource |        |
| Log.LogLevel   | ""                                                           | slime log level                                              |        |
| Log.KlogLevel  | 0                                                            | klog log level                                               |        |
| Misc           | {"metrics-addr": ":8080", "aux-addr": ":8081", "enable-leader-election": "off", "global-sidecar-mode": "namespace"}, | Scalable collection map. It contains 3 params now: 1. "metrics-addr", metrics address of slime module manager; 2."aux-addr", address of auxiliary web server; 3."enable-leader-election", switch of enabling leader election of slime module controller; 4."namespace-global-sidecar", global-sidecar using mode, default is "namespace", others are "cluster", "no" |        |



### Replacement Method

For the replacement of values.yaml, follow the syntax rules of helm [Values file](https://helm.sh/zh/docs/chart_template_guide/values_files/). According to the using way of slime/slime-boot/helm-charts/slimeboot/templates, add the fields to be overwritten to the corresponding location in the slimeboot cr resource.

For the replacement of Config.global, add the fields to be overwritten to the corresponding location in the slimeboot cr spec.module.global. 

#### Example

The following example customizes the namespace deployed slime, resources size, etc. to replace some default values for modules and components in slime.

```yaml
---
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: slime					#customize the namespace deployed slime
spec:
  namespace: slime					#customize the namespace deployed slime, same with config.global.slimeNamespace
  istioNamespace: istio-system		#customize the namespace deployed istio, same with config.global.istioNamespace
  healthProbePort: 9091				#same with the port contained by config.global.misc["aux-addr"]
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-lazyload
    tag: v0.2.2
  module:
    - name: lazyload
      enable: true
      fence:
        wormholePort:
          - "9080"
      global:
        slimeNamespace: slime		#customize the slime deployed namespace filled in sidecar, same with spec.namespace
        istioNamespace: istio-system 	#customize the istio deployed namespace filled in sidecar, same with spec.istioNamespace
        log:						#customize log level
          logLevel: debug
          klogLevel: 10        
        misc:
          metrics-addr: ":9090"		#customize the address of slime module manager
          aux-addr: ":9091"			#customize auxiliary http server address, same with spec.healthProbePort
      metric:
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
      type: namespaced
      namespace:
        - default
      resources:		#customize resources
        requests:	
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 200m
          memory: 200Mi
      image:
        repository: istio/proxyv2
        tag: 1.7.0          
    pilot:
      enable: true
      resources:		#customize resources
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 200m
          memory: 200Mi
      image:
        repository: docker.io/slimeio/pilot
        tag: globalPilot-7.0-v0.0.3-833f1bd5c1
```



