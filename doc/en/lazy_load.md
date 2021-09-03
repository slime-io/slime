- [Install & Use](#install--use)
- [Other installation options](#other-installation-options)
  - [Disable global-sidecar](#disable-global-sidecar)
  - [Use cluster unique global-sidecar](#use-cluster-unique-global-sidecar)
  - [Use report-server to report the dependency](#use-report-server-to-report-the-dependency)
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

#### Install & Use

Make sure slime-boot has been installed.

1. Install the lazyload module and additional components, through slime-boot configuration:

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
        - {{your_namespace}} # replace to your deployment's namespace, and extend the list in case of multi namespaces
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
        tag: {{your_pilot_tag}}
```

[Example](../../install/samples/lazyload/slimeboot_lazyload.yaml)

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

#### Other installation options

##### Disable global-sidecar  

In the ServiceMesh with allow_any enabled, the global-sidecar component can be omitted. Use the following configuration:

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
      metric:
        prometheus:
          address: {{prometheus_address}} # replace to your prometheus address
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",reporter="destination"})by(destination_service)
              type: Group
```

[Example](../../install/samples/lazyload/slimeboot_lazyload_no_global_sidecar.yaml)

Not using the global-sidecar component may cause the first call to fail to follow the preset traffic rules.



##### Use cluster unique global-sidecar     

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
    pilot:
      enable: true
      image:
        repository: docker.io/slimeio/pilot
        tag: {{your_pilot_tag}}     
```

[Example](../../install/samples/lazyload/slimeboot_lazyload_cluster_global_sidecar.yaml)



##### Use report-server to report the dependency   

When prometheus is not configured in the cluster, the dependency can be reported through report-server.  Add reportServer in component, and set reportServer.enable = true.

If using prometheus, delete reportServer from component, or set reportServer.enable = false.

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
  # Default values copied from <project_dir>/helm-charts/slimeboot/values.yaml\
  module:
    - fence:
        enable: true
        wormholePort:
        - "{{your_port}}" # replace to your application service ports, and extend the list in case of multi ports
      name: slime-fence
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
        - {{your_namespace}} # replace to your deployment's namespace, and extend the list in case of multi namespaces
    pilot:
      enable: true
      image:
        repository: docker.io/slimeio/pilot
        tag: {{your_pilot_tag}}
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
        tag: {{your_mixer_image}}
      inspectorImage:
        repository: docker.io/slimeio/report-server
        tag: {{your_inspector_image}}        
```

[Example](../../install/samples/lazyload/slimeboot_lazyload_reportserver.yaml)



#### Example

##### Install Istio (1.8+)



##### Set Tag

$latest_tag equals the latest tag. The shell scripts and yaml files uses this version as default.

```sh
$ export latest_tag=$(curl -s https://api.github.com/repos/slime-io/slime/tags | grep 'name' | cut -d\" -f4 | head -1)
```



##### Install Slime

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



##### Install Bookinfo

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



##### Enable Lazyload

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



##### First Visit and Observ

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



##### Second Visit and Observ

Visit the productpage website again, and use `kubectl logs -f productpage-xxx -c istio-proxy -n default` to observe the access log of productpage.

```
[2021-08-06T06:05:47.068Z] "GET /details/0 HTTP/1.1" 200 - via_upstream - "-" 0 178 46 46 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "1c1c8e23-24d3-956e-aec0-e4bcff8df251" "details:9080" "172.17.0.6:9080" outbound|9080||details.default.svc.cluster.local 172.17.0.11:58522 10.101.207.55:9080 172.17.0.11:57528 - default
[2021-08-06T06:05:47.160Z] "GET /reviews/0 HTTP/1.1" 200 - via_upstream - "-" 0 379 1559 1558 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36" "1c1c8e23-24d3-956e-aec0-e4bcff8df251" "reviews:9080" "172.17.0.10:9080" outbound|9080||reviews.default.svc.cluster.local 172.17.0.11:60104 10.106.126.147:9080 172.17.0.11:42280 - default
```

The backends are details and reviews now.



##### Uninstall

Uninstall bookinfo.

```sh
$ kubectl delete -f "https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/config/bookinfo.yaml"
```

Uninstall slime.

```sh
$ /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/slime-io/slime/$latest_tag/install/samples/lazyload/easy_uninstall_lazyload.sh)"
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

