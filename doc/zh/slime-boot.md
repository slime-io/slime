- [安装slime-boot](#安装slime-boot)
- [安装Prometheus](#安装prometheus)
- [验证](#验证)
- [slimeboot默认值说明与替换方法](#slimeboot默认值说明与替换方法)
  - [默认值说明](#默认值说明)
    - [values.yaml](#valuesyaml)
    - [Config.global](#configglobal)
  - [替换方法](#替换方法)
    - [样例](#样例)



## 安装slime-boot

在使用slime module之前，需要安装deployment/slime-boot，实际是一个封装的helm operator。它会监听slimeboot cr资源的创建，可以方便的安装和卸载slime模块。 

此处$tag_or_commit使用最新tag。如有需要，也可以自行替换为老版本tag或commit_id。执行如下命令：

```shell
$ export tag_or_commit=$(curl -s https://api.github.com/repos/slime-io/slime/tags | grep 'name' | cut -d\" -f4 | head -1)
$ kubectl create ns mesh-operator
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/init/crds.yaml"
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/init/deployment_slime-boot.yaml"
```



## 准备Metric Source

支持的指标来源有Prometheus和Accesslog。Slime的懒加载(Lazyload)和自适应限流(Limiter)等模块运行时需要监控指标。具体来说，懒加载支持Promehteus或Accesslog，自适应限流支持Prometheus。

如果需要从Prometheus获取指标，这里提供一份istio官网的Prometheus简化部署文件，可以一键部署出Prometheus。

```shell
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$tag_or_commit/install/config/prometheus.yaml"
```



## 验证

安装完毕后，检查mesh-operator中创建的slime-boot与istio-system中的prometheus。如果是Accesslog，则没有额外组件。

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



## slimeboot默认值说明与替换方法

### 默认值说明

默认值来源分为两块

1. 部署参数：slime-boot operator中的values.yaml文件，文件路径slime/slime-boot/helm-charts/slimeboot/values.yaml。这部分值都是和“k8s部署”直接相关的，比如replicaCount，serviceAccount，imagePullPolicy等
2. 运行时参数：slime framework中defaultModuleConfig变量的Global部分，文件路径slime/framework/bootstrap/config.go。这部分值都用于Go程序运行。

部署参数和运行时参数是有交集的，比如对于健康检查来说，部署时指定的端口`healthProbePort`，要等于运行时程序暴露的端口`aux-addr`；再比如，启用日志输出到本地文件时，存储卷挂载路径`volumeMounts.mountPath`要与运行时logger输出的文件路径`Config.Global.Log.LogRotateConfig.FilePath`匹配。



#### values.yaml

slimeboot operator templates用到的所有默认值介绍，会用来创建slime module和slime component。

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
| healthProbePort                            | 8081          | module                                                      | 如果修改，要和config.global.misc["aux-addr"]包含的端口值一致 |
| logSourcePort                              | 8082          | module                                                      | module deployment接收accesslog的grpc端口                     |
| service.logSourcePort                      | 8082          | module                                                      | module service接收accesslog的grpc端口，要与logSourcePort保持一致 |
| containers.slime.volumeMounts              | ""            | module                                                      | 启用日志轮转时，存储卷对应的本地路径                         |
| volumes                                    | ""            | module                                                      | 启用日志轮转时，使用的存储卷信息                             |



#### Config.global

主要是slime module会用到的一些配置，与slime component创建无关。

| Key                            | Default Value                                                | Usages                                                       | Remark |
| ------------------------------ | ------------------------------------------------------------ | ------------------------------------------------------------ | ------ |
| Service                        | app                                                          | servicefence匹配服务的label key，用来生成懒加载中sidecar的默认配置 |        |
| IstioNamespace                 | istio-system                                                 | 部署istio组件的namespace，用来生成懒加载中sidecar的默认配置，应等于实际部署istio组件的namespace |        |
| SlimeNamespace                 | mesh-operator                                                | 部署slime模块的namespace，用来生成懒加载中sidecar的默认配置，应等于实际创建slimeboot cr资源的namespace |        |
| Log.LogLevel                   | ""                                                           | slime自身日志级别                                            |        |
| Log.KlogLevel                  | 0                                                            | klog日志级别                                                 |        |
| Log.LogRotate                  | false                                                        | 是否启用日志轮转，即日志输出本地文件                         |        |
| Log.LogRotateConfig.FilePath   | "/tmp/log/slime.log"                                         | 本地日志文件路径                                             |        |
| Log.LogRotateConfig.MaxSizeMB  | 100                                                          | 本地日志文件大小上限，单位MB                                 |        |
| Log.LogRotateConfig.MaxBackups | 10                                                           | 本地日志文件个数上限                                         |        |
| Log.LogRotateConfig.MaxAgeDay  | 10                                                           | 本地日志文件保留时间，单位天                                 |        |
| Log.LogRotateConfig.Compress   | false                                                        | 本地日志文件轮转后是否压缩                                   |        |
| Misc                           | {"metrics-addr": ":8080", "aux-addr": ":8081", "enable-leader-election": "off","global-sidecar-mode": "namespace","metric_source_type": "prometheus","log_source_port": ":8082"}, | 可扩展的配置集合，目前有六个参数：1."metrics-addr"定义slime module manager监控指标暴露地址；2."aux-addr"定义辅助服务器暴露地址；3."enable-leader-election"定义manager是否启用选主功能；4."global-sidecar-mode"定义global-sidecar的使用模式，默认是"namespace"，可选的还有"cluster", "no"；5."metric_source_type"定义监控指标来源，默认是"prometheus"，可选"accesslog"；6."log_source_port"定义使用accesslog做指标源时，接收accesslog的端口，默认是8082，如果要修改，注意也要修改helm模板中的logSourcePort |        |



### 替换方法

对于values.yaml的替换，按照helm的语法规则[Values file](https://helm.sh/zh/docs/chart_template_guide/values_files/)。可以根据slime/slime-boot/helm-charts/slimeboot/templates中的使用方式，添加需要覆盖的字段到slimeboot cr资源的相应位置。

对于Config.global的替换，添加需要覆盖的字段到slimeboot cr资源的spec.module.global中相应位置。

#### 样例

下面的例子自定义了slime部署的namespace、resources资源大小等内容，从而实现slime中module和component的默认值替换。

```yaml
---
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: slime
spec:
  namespace: slime					#自定义slime部署的namespace，和config.global.slimeNamespace一致
  istioNamespace: istio-system		#自定义istio部署的namespace，和config.global.istioNamespace一致
  healthProbePort: 9091				#和config.global.misc["aux-addr"]包含的端口值一致
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-lazyload
    tag: v0.2.6-db8a720-dirty
  module:
    - name: lazyload
      enable: true
      fence:
        wormholePort:
          - "9080"
      global:
        slimeNamespace: slime		#自定义sidecar默认配置中部署slime的namespace，与spec.namespace保持一致
        istioNamespace: istio-system	#自定义sidecar默认配置中部署istio的namespace，与spec.istioNamespace保持一致
        log:						#自定义log级别
          logLevel: debug
          klogLevel: 10
        misc:
          metrics-addr: ":9090"		#自定义slime module manager监控指标暴露地址
          aux-addr: ":9091"			#自定义辅助服务器暴露地址，与spec.healthProbePort一致
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
      resources:		#自定义resources
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
      resources:		#自定义resources
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

