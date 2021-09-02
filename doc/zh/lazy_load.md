- [安装和使用](#安装和使用)
- [其他安装选项](#其他安装选项)
- [示例: 为bookinfo的productpage服务开启懒加载](#示例-为bookinfo的productpage服务开启懒加载)
  - [安装 istio (1.8+)](#安装-istio-18)
  - [安装 slime](#安装-slime)
  - [安装bookinfo](#安装bookinfo)
  - [开启懒加载](#开启懒加载)
  - [首次访问观察](#首次访问观察)
  - [再次访问观察](#再次访问观察)
  - [卸载](#卸载)

#### 安装和使用

请先按照安装slime-boot小节的指引安装slime-boot

1. 使用Slime的配置懒加载功能需打开Fence模块，同时安装global-sidecar, pilot等附加组件，如下：

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
    tag: v0.2.0-alpha
  module:
    - name: lazyload
      fence:
        enable: true
        wormholePort: 
          - "9080" # replace to your application service ports
          - {{your port}}
      metric:
        prometheus:
          address: http://prometheus.istio-system:9090 # replace to your prometheus address
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
        - default # replace to or add your deployment's namespace
        - {{you namespace}}
      resources:
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 200m
          memory: 200Mi
    pilot:
      enable: true
      resources:
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 200m
          memory: 200Mi
      image:
        repository: docker.io/slimeio/pilot
        tag: global-pilot-v0.0.2-a85b00
```

2. 确认所有组件已正常运行：

```
$ kubectl get po -n mesh-operator
NAME                                    READY     STATUS    RESTARTS   AGE
global-sidecar-pilot-796fb554d7-blbml   1/1       Running   0          27s
lazyload-fbcd5dbd9-jvp2s                1/1       Running   0          27s
slime-boot-68b6f88b7b-wwqnd             1/1       Running   0          39s
```

```
$ kubectl get po -n {{your namespace}}
NAME                              READY     STATUS    RESTARTS   AGE
global-sidecar-785b58d4b4-fl8j4   1/1       Running   0          68s
```

3. 打开配置懒加载：
   业务namespace已有应用，在业务namespace中创建servicefence，执行`kubectl apply -f servicefence.yaml`

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: ServiceFence
metadata:
  name: {{your svc}}
  namespace: {{you namespace}}
spec:
  enable: true
```

4. 确认懒加载已开启
   执行`kubectl get sidecar {{your svc}} -oyaml`，可以看到对应服务生成了一个sidecar，如下：

```yaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  name: {{your svc}}
  namespace: {{you namespace}}
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ServiceFence
    name: {{your svc}}
spec:
  egress:
  - hosts:
    - istio-system/*
    - mesh-operator/*
    - '*/global-sidecar.{{your ns}}.svc.cluster.local'
  workloadSelector:
    labels:
      app: {{your svc}}
```



#### 其他安装选项

**不使用global-sidecar组件**  
在开启allow_any的网格中，可以不使用global-sidecar组件。使用如下配置：

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
    tag: v0.2.0-alpha
  module:
    - fence:
        enable: true
        wormholePort:
        - {{port1}} # replace to your application service ports
        - {{port2}}
        - ...
      name: slime-fence
      metric:
        prometheus:
          address: http://prometheus.istio-system:9090 # replace to your prometheus address
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",reporter="destination"})by(destination_service)
              type: Group
```

不使用global-sidecar组件可能会导致首次调用无法按照预先设定的路由规则进行。   

**使用集群唯一的global-sidecar**   

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
    tag: v0.2.0-alpha
  module:
    - fence:
        enable: true
        wormholePort:
        - {{port1}} # replace to your application service ports
        - {{port2}}
        - ...
      name: slime-fence
      metric:
        prometheus:
          address: http://prometheus.istio-system:9090 # replace to your prometheus address
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",reporter="destination"})by(destination_service)
              type: Group
  component:
    globalSidecar:
      enable: true
      type: cluster
      namespace:
        - default # replace to or add your deployment's namespace
        - {{you namespace}}
    pilot:
      enable: true
      image:
        repository: docker.io/slimeio/pilot
        tag: global-pilot-v0.0.2-a85b00      
```

**使用report-server上报调用关系**   
集群内未配置prometheus时，可通过report-server上报依赖关系   

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
    tag: v0.2.0-alpha
  # Default values copied from <project_dir>/helm-charts/slimeboot/values.yaml\
  module:
    - fence:
        enable: true
        wormholePort:
        - {{port1}} # replace to your application service ports 
        - {{port2}}
        - ...
      name: slime-fence
      metric:
        prometheus:
          address: http://prometheus.istio-system:9090 # replace to your prometheus address
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
        - default # replace to your deployment's namespace
        - {{you namespace}}
    pilot:
      enable: true
      image:
        repository: docker.io/slimeio/pilot
        tag: global-pilot-v0.0.2-a85b00
    reportServer:
      enable: true
      resources:
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 200m
          memory: 200Mi
      mixerImage:
        repository: docker.io/slimeio/mixer
        tag: preview-1.3.7-v0.0.1
      inspectorImage:
        repository: docker.io/slimeio/report-server
        tag: preview-v0.0.1-rc    
```





#### 示例: 为bookinfo的productpage服务开启懒加载

##### 安装 istio (1.8+)



##### 安装 slime 

```shell
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/samples/lazyload/easy_install_lazyload.sh)"
```

确认所有组件已正常运行

```sh
$ kubectl get slimeboot -n mesh-operator
NAME       AGE
lazyload   2m20s
$ kubectl get pod -n mesh-operator
NAME                                    READY   STATUS             RESTARTS   AGE
global-sidecar-pilot-7bfcdc55f6-977k2   1/1     Running            0          2m25s
lazyload-b9646bbc4-ml5dr                1/1     Running            0          2m25s
slime-boot-7b474c6d47-n4c9k             1/1     Running            0          4m55s
$ kubectl get po -n default
NAME                              READY   STATUS    RESTARTS   AGE
global-sidecar-59f4c5f989-ccjjg   1/1     Running   0          3m9s
```



##### 安装bookinfo

   创建前请将current-context中namespace切换到你想部署bookinfo的namespace，使bookinfo创建在其中。此处以default为例。

```sh
$ kubectl label namespace default istio-injection=enabled
$ kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/config/bookinfo.yaml
```

创建完后，状态如下

```sh
$ kubectl get po -n default
NAME                              READY   STATUS    RESTARTS   AGE
details-v1-79f774bdb9-6vzj6       2/2     Running   0          60s
global-sidecar-59f4c5f989-ccjjg   1/1     Running   0          5m12s
productpage-v1-6b746f74dc-vkfr7   2/2     Running   0          59s
ratings-v1-b6994bb9-klg48         2/2     Running   0          59s
reviews-v1-545db77b95-z5ql9       2/2     Running   0          59s
reviews-v2-7bf8c9648f-xcvd6       2/2     Running   0          60s
reviews-v3-84779c7bbc-gb52x       2/2     Running   0          60s
```

此样例中可以在pod/ratings中发起对productpage的访问，`curl productpage:9080/productpage`。另外也可参考 https://istio.io/latest/zh/docs/setup/getting-started/#ip 给应用暴露外访接口。



##### 开启懒加载

创建servicefence，为productpage服务启用懒加载。

```sh
$ kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/samples/lazyload/servicefence_productpage.yaml
```

确认生成servicefence和sidecar对象。

```sh
$ kubectl get servicefence -n default
NAME          AGE
productpage   12s
$ kubectl get sidecar -n default
NAME          AGE
productpage   22s
$ kubectl get sidecar productpage -n default -oyaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  creationTimestamp: "2021-08-04T03:54:35Z"
  generation: 1
  name: productpage
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ServiceFence
    name: productpage
    uid: d36e4be7-d66c-4f77-a9ff-14a4bf4641e6
  resourceVersion: "324118"
  uid: ec283a14-8746-42d3-87d1-0ee4538f0ac0
spec:
  egress:
  - hosts:
    - istio-system/*
    - mesh-operator/*
    - '*/global-sidecar.default.svc.cluster.local'
  workloadSelector:
    labels:
      app: productpage
```


##### 首次访问观察

第一次访问productpage，并使用`kubectl logs -f productpage-xxx -c istio-proxy -n default`观察访问日志。

```
[2021-08-06T06:04:36.912Z] "GET /details/0 HTTP/1.1" 200 - via_upstream - "-" 0 178 43 43 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "48257260-1f5f-92fa-a18f-ff8e2b128487" "details:9080" "172.17.0.17:9080" outbound|9080||global-sidecar.default.svc.cluster.local 172.17.0.11:45422 10.101.207.55:9080 172.17.0.11:56376 - -
[2021-08-06T06:04:36.992Z] "GET /reviews/0 HTTP/1.1" 200 - via_upstream - "-" 0 375 1342 1342 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "48257260-1f5f-92fa-a18f-ff8e2b128487" "reviews:9080" "172.17.0.17:9080" outbound|9080||global-sidecar.default.svc.cluster.local 172.17.0.11:45428 10.106.126.147:9080 172.17.0.11:41130 - -
```

可以看出，此次outbound后端访问global-sidecar.default.svc.cluster.local。

观察sidecar内容

```sh
$ kubectl get sidecar productpage -oyaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  creationTimestamp: "2021-08-06T03:23:05Z"
  generation: 2
  name: productpage
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ServiceFence
    name: productpage
    uid: 27853fe0-01b3-418f-a785-6e49db0d201a
  resourceVersion: "498810"
  uid: e923e426-f0f0-429a-a447-c6102f334904
spec:
  egress:
  - hosts:
    - '*/details.default.svc.cluster.local'
    - '*/reviews.default.svc.cluster.local'
    - istio-system/*
    - mesh-operator/*
    - '*/global-sidecar.default.svc.cluster.local'
  workloadSelector:
    labels:
      app: productpage
```

reviews 和 details 被自动加入！



##### 再次访问观察

第二次访问productpage，观察productpage应用日志

```
[2021-08-06T06:05:47.068Z] "GET /details/0 HTTP/1.1" 200 - via_upstream - "-" 0 178 46 46 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "1c1c8e23-24d3-956e-aec0-e4bcff8df251" "details:9080" "172.17.0.6:9080" outbound|9080||details.default.svc.cluster.local 172.17.0.11:58522 10.101.207.55:9080 172.17.0.11:57528 - default
[2021-08-06T06:05:47.160Z] "GET /reviews/0 HTTP/1.1" 200 - via_upstream - "-" 0 379 1559 1558 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "1c1c8e23-24d3-956e-aec0-e4bcff8df251" "reviews:9080" "172.17.0.10:9080" outbound|9080||reviews.default.svc.cluster.local 172.17.0.11:60104 10.106.126.147:9080 172.17.0.11:42280 - default
```

可以看到，outbound日志的后端访问信息变为details.default.svc.cluster.local和reviews.default.svc.cluster.local。



##### 卸载

卸载bookinfo

```sh
$ kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/config/bookinfo.yaml
```

卸载slime相关

```sh
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/samples/lazyload/easy_uninstall_lazyload.sh)"
```



