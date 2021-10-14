- [Install & Use](#install--use)
- [Other installation options](#other-installation-options)
  - [Disable global-sidecar](#disable-global-sidecar)
  - [Use cluster unique global-sidecar](#use-cluster-unique-global-sidecar)
- [Introduction of features](#introduction-of-features)
  - [Automatic ServiceFence generation based on namespace/service label](#automatic-servicefence-generation-based-on-namespaceservice-label)
  - [Custom undefined traffic dispatch](#custom-undefined-traffic-dispatch)
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

[TOC]


## Install & Use

Make sure slime-boot has been installed.

1. Install the lazyload module and additional components, through slime-boot configuration:

   > [Example](../../install/samples/lazyload/slimeboot_lazyload.yaml)

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
    tag: {{your_lazyload_tag}}
  module:
    - name: lazyload
      fence:
        enable: true
        wormholePort: 
          - "{{your_port}}" # replace to your application service ports, and extend the list in case of multi ports
      metric:
        prometheus:
          address: {{prometheus_address}} # replace to your prometheus address
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
        - {{your_namespace}} # replace to your service's namespace, and extend the list in case of multi namespaces
      resources:
        requests:
          cpu: 200m
          memory: 200Mi
        limits:
          cpu: 200m
          memory: 200Mi
      image:
        repository: {{your_sidecar_repo}}
        tag: {{your_sidecar_tag}}           
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
        repository: {{your_pilot_repo}}
        tag: {{your_pilot_tag}}
```



2. make sure all components are running

```sh
$ kubectl get po -n mesh-operator
NAME                                    READY     STATUS    RESTARTS   AGE
global-sidecar-pilot-796fb554d7-blbml   1/1       Running   0          27s
lazyload-fbcd5dbd9-jvp2s                1/1       Running   0          27s
slime-boot-68b6f88b7b-wwqnd             1/1       Running   0          39s
```

```sh
$ kubectl get po -n {{your_namespace}}
NAME                              READY     STATUS    RESTARTS   AGE
global-sidecar-785b58d4b4-fl8j4   1/1       Running   0          68s
```

3. enable lazyload    

  Apply servicefence resource to enable lazyload.

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: ServiceFence
metadata:
  name: {{your_svc}}
  namespace: {{your_namespace}}
spec:
  enable: true
```

4. make sure sidecar has been generated
   Execute `kubectl get sidecar {{svc name}} -oyaml`，you can see a sidecar is generated for the corresponding service， as follow：

```yaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  name: {{your_svc}}
  namespace: {{your_ns}}
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ServiceFence
    name: {{your_svc}}
spec:
  egress:
  - hosts:
    - istio-system/*
    - mesh-operator/*
    - '*/global-sidecar.{your ns}.svc.cluster.local'
  workloadSelector:
    labels:
      app: {{your_svc}}
```

## Other installation options

### Disable global-sidecar  

In the ServiceMesh with allow_any enabled, the global-sidecar component can be omitted. Use the following configuration:

> [Example](../../install/samples/lazyload/slimeboot_lazyload_no_global_sidecar.yaml)
>
> Instructions: 
>
> Not using the global-sidecar component may result in the first call not following the pre-defined routing rules. It may result in the underlying logic of istio (typically passthrough), then it come back to send request using clusterIP. VirtualService temporarily disabled.
>
> Scenario: 
>
> Service A accesses service B, but service B's virtualservice directs the request for access to service B to service C. Since there is no global sidecar to handle this, the first request is transmitted by istio to service B via PassthroughCluster. What should have been a response from service C becomes a response from service B with an error. After first request, B adds to A's servicefence, then A senses that the request is directed to C by watching B's virtualservice. Later C adds to A's servicefence., and all requests after the first time will be successfully responded by C.
>

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
    tag: {{your_lazyload_tag}}
  module:
    - fence:
        enable: true
        wormholePort:
        - "{{your_port}}" # replace to your application service ports, and extend the list in case of multi ports
      name: slime-fence
      global:
        misc:
          global-sidecar-mode: no      
      metric:
        prometheus:
          address: {{prometheus_address}} # replace to your prometheus address
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",reporter="destination"})by(destination_service)
              type: Group
```



### Use cluster unique global-sidecar   

> [Example](../../install/samples/lazyload/slimeboot_lazyload_cluster_global_sidecar.yaml)  
>
> Instructions: 
>
> In k8s, the traffic of short domain access will only come from the same namespace, and cross-namespace access must carry namespace information. Cluster unique global-sidecar is often not under the same namespace with business service, so its envoy config lacks the configuration of short domain. Therefore, cluster unique global-sidecar cannot successfully forward access requests within the same namespace, resulting in timeout "HTTP/1.1 0 DC downstream_remote_disconnect" error. 
>
> So in this case, inter-application access should carry namespace information.

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
    tag: {{your_lazyload_tag}}
  module:
    - fence:
        enable: true
        wormholePort:
        - "{{your_port}}" # replace to your application service ports, and extend the list in case of multi ports
      name: slime-fence
      global:
        misc:
          global-sidecar-mode: cluster      
      metric:
        prometheus:
          address: {{prometheus_address}} # replace to your prometheus address
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",reporter="destination"})by(destination_service)
              type: Group
  component:
    globalSidecar:
      enable: true
      type: cluster
      image:
        repository: {{your_sidecar_repo}}
        tag: {{your_sidecar_tag}}      
    pilot:
      enable: true
      image:
        repository: {{your_pilot_repo}}
        tag: {{your_pilot_tag}}     
```





## Introduction of features

### Automatic ServiceFence generation based on namespace/service label



fence supports automatic generation based on label, i.e. you can define  **the scope of "fence enabled" functionality** by typing label `slime.io/serviceFenced`.

* namespace level

  * `true`: Servicefence cr will be created for all services (without cr) under this namespace 
  * Other values: No action

* service level

  * `true`: generates servicefence cr for this service
  * `false`: do not generate servicefence cr for this service

  > All of the above will override the namespace level setting (label)

  * other values: use namespace level configuration



For automatically generated servicefence cr, it will be recorded by the standard label `app.kubernetes.io/created-by=fence-controller`, which implements the state association change. Servicefence that do not match this label are currently considered manually configured and are not affected by the above labels.



**Example**

> namespace `testns` has three services under it: `svc1`, `svc2`, `svc3`

* Label `testns` with `slime.io/serviceFenced=true`: Generate cr for the above three services
* Label `svc2` with `slime.io/serviceFenced=false`: only the cr for `svc1`, `svc3` remain
* Remove this label from `svc2`: restores three cr
* Remove `app.kubernetes.io/created-by=fence-controller` from the cr of `svc3`; remove the label on `testns`: only the cr of `svc3` remains



**Sample configuration**

```yaml
apiVersion: v1
kind: Namespace
metadata:
  creationTimestamp: "2021-03-16T09:36:25Z"
  labels:
    istio-injection: enabled
    slime.io/serviceFenced: "true"
  name: testns
  resourceVersion: "79604437"
  uid: 5a34b780-cd95-4e43-b706-94d89473db77
---
apiVersion: v1
kind: Service
metadata:
  annotations: {}
  labels:
    app: svc2
    service: svc2
    slime.io/serviceFenced: "false"
  name: svc2
  namespace: testns
  resourceVersion: "79604741"
  uid: b36f04fe-18c6-4506-9d17-f91a81479dd2
```





### Custom undefined traffic dispatch

By default, lazyload/fence sends  (default or undefined) traffic that envoy cannot match the route to the global sidecar to deal with the problem of missing service data temprorarily, which is inevitably faced by "lazy loading". This solution is limited by technical details, and cannot handle traffic whose target (e.g. domain name) is outside the cluster, see [[Configuration Lazy Loading]: Failed to access external service #3](https://github.com/slime-io/) slime/issues/3).

Based on this background, this feature was designed to be used in more flexible business scenarios as well. The general idea is to assign different default traffic to different targets for correct processing by means of domain matching.



Sample configuration.

```yaml
module:
  - name: fence
    fence:
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

foo: bar
```

> In this example, we dispatch a portion of the traffic to the specified cluster; let another part go to the global sidecar; and then for the rest of the traffic, let it keep the native istio behavior: passthrough.



**Note**:

* In custom assignment scenarios, if you want to keep the original logic "all other undefined traffic goes to global sidecar", you need to explicitly configure the last item as above



## Example

### Install Istio (1.8+)



### Set Tag

$latest_tag equals the latest tag. The shell scripts and yaml files uses this version as default.

```sh
$ export latest_tag=$(curl -s https://api.github.com/repos/slime-io/slime/tags | grep 'name' | cut -d\" -f4 | head -1)
```



### Install Slime

```sh
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/samples/lazyload/easy_install_lazyload.sh)"
```

Confirm all components are running.

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



### Install Bookinfo

Change the namespace of current-context to which bookinfo will deploy first. Here we use default namespace.

```sh
$ kubectl label namespace default istio-injection=enabled
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/config/bookinfo.yaml"
```

Confirm all pods are running.

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

Then we can visit productpage from pod/ratings, executing `curl productpage:9080/productpage`. 

You can also create gateway and visit productpage from outside, like what shows in  [Open the application to outside traffic](https://istio.io/latest/docs/setup/getting-started/#ip).



### Enable Lazyload

Create lazyload for productpage.

```sh
$ kubectl apply -f "https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/samples/lazyload/servicefence_productpage.yaml"
```

Confirm servicefence and sidecar already exist.

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



### First Visit and Observ

Visit the productpage website, and use `kubectl logs -f productpage-xxx -c istio-proxy -n default` to observe the access log of productpage.

```
[2021-08-06T06:04:36.912Z] "GET /details/0 HTTP/1.1" 200 - via_upstream - "-" 0 178 43 43 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "48257260-1f5f-92fa-a18f-ff8e2b128487" "details:9080" "172.17.0.17:9080" outbound|9080||global-sidecar.default.svc.cluster.local 172.17.0.11:45422 10.101.207.55:9080 172.17.0.11:56376 - -
[2021-08-06T06:04:36.992Z] "GET /reviews/0 HTTP/1.1" 200 - via_upstream - "-" 0 375 1342 1342 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "48257260-1f5f-92fa-a18f-ff8e2b128487" "reviews:9080" "172.17.0.17:9080" outbound|9080||global-sidecar.default.svc.cluster.local 172.17.0.11:45428 10.106.126.147:9080 172.17.0.11:41130 - -
```

It is clearly that the banckend of productpage is global-sidecar.

Now we get the sidecar yaml. 

```YAML
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

Details and reviews are already added into sidecar! 



### Second Visit and Observ

Visit the productpage website again, and use `kubectl logs -f productpage-xxx -c istio-proxy -n default` to observe the access log of productpage.

```
[2021-08-06T06:05:47.068Z] "GET /details/0 HTTP/1.1" 200 - via_upstream - "-" 0 178 46 46 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "1c1c8e23-24d3-956e-aec0-e4bcff8df251" "details:9080" "172.17.0.6:9080" outbound|9080||details.default.svc.cluster.local 172.17.0.11:58522 10.101.207.55:9080 172.17.0.11:57528 - default
[2021-08-06T06:05:47.160Z] "GET /reviews/0 HTTP/1.1" 200 - via_upstream - "-" 0 379 1559 1558 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "1c1c8e23-24d3-956e-aec0-e4bcff8df251" "reviews:9080" "172.17.0.10:9080" outbound|9080||reviews.default.svc.cluster.local 172.17.0.11:60104 10.106.126.147:9080 172.17.0.11:42280 - default
```

The backends are details and reviews now.



### Uninstall

Uninstall bookinfo.

```sh
$ kubectl delete -f "https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/config/bookinfo.yaml"
```

Uninstall slime.

```sh
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/samples/lazyload/easy_uninstall_lazyload.sh)"
```



### Remarks

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

