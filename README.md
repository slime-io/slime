# Slime

Slime is a CRD controller for istio. Designed to use istio/envoy advanced functions more automatically and conveniently through simple configuration. Currently slime contains three sub-modules:

**[Configuration Lazy Loading](#configure-lazy-loading):** No need to configure SidecarScope, automatically load configuration/service discovery information on demand

**[Http Plugin Management](#http-plugin-management):** Use the new CRD pluginmanager/envoyplugin to wrap readability , The poor maintainability of envoyfilter makes plug-in extension more convenient

**[Adaptive Ratelimit](#adaptive-ratelimit):** Local current limiting is realized, and it can be automatically combined with monitoring information Adjust current limiting strategy

## Install slime-boot
You can easily install and uninstall the slime sub-module with slime-boot. Using the following commands to install slime-boot:
```
kubectl create ns mesh-operator
kubectl apply -f https://raw.githubusercontent.com/ydh926/slime/master/install/crds.yaml
kubectl apply -f https://raw.githubusercontent.com/ydh926/slime/master/install/slime-boot-install.yaml
```

## Configure lazy loading
### Install & Use

**make sure [slime-boot](#install-slime-boot) has been installed.**
  
1. Install the lazyload module and additional components, through slime-boot configuration:
```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  # Default values copied from <project_dir>/helm-charts/slimeboot/values.yaml\
  module:
    - fence:
        enable: true
        wormholePort:
        - {{port1}} # application svc port
        - {{port2}}
        - ...
      name: slime-fence
 component:
   globalSidecar:
     enable: true
     namespace:
       - {{you namespace}} # application namespaces
   pilot:
     enable: true
     image:
       repository: docker.io/bcxq/pilot
       tag: preview-1.3.7-v0.0.1
   reportServer:
     enable: true
     image:
       repository: docker.io/bcxq/mixer
       tag: preview-1.3.7-v0.0.1  
```
2. make sure all components are running
```
$ kubectl get po -n mesh-operator
NAME                                    READY     STATUS    RESTARTS   AGE
global-sidecar-pilot-796fb554d7-blbml   1/1       Running   0          27s
lazyload-fbcd5dbd9-jvp2s                1/1       Running   0          27s
report-server-855c8cf558-wdqjs          2/2       Running   0          27s
slime-boot-68b6f88b7b-wwqnd             1/1       Running   0          39s
```
```
$ kubectl get po -n {{your namespace}}
NAME                              READY     STATUS    RESTARTS   AGE
global-sidecar-785b58d4b4-fl8j4   1/1       Running   0          68s
```

3. enable lazyload
```shell
kubectl label ns {{your namespace}} istio-dependency-servicefence=true
```
```shell
kubectl annotate svc {{your svc}} istio.dependency.servicefence/status=true
```

4. make sure SidecarScope has been generated
Execute `kubectl get sidecar {{svc name}} -oyaml`，you can see a sidecar is generated for the corresponding service， as follow：
```yaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  name: {{your svc}}
  namespace: {{your ns}}
  ownerReferences:
  - apiVersion: microservice.netease.com/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ServiceFence
    name: {{your svc}}
spec:
  egress:
  - hosts:
    - istio-system/*
    - mesh-operator/*
    - '*/global-sidecar.{your ns}.svc.cluster.local'
  workloadSelector:
    labels:
      app: {{your svc}}
```

### Uninstall

1. Delete corresponding slime-boot configuration.
2. Delete all servicefence configurations.
```shell
for i in $(kubectl get ns);do kubectl delete servicefence -n $i --all;done
```

### Example

1. install istio ( > 1.8 )
2. install slime 
```shell
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/ydh926/slime/master/install/easy_install_lazyload.sh)"
```
3. Make sure all components are running
```
$ kubectl get po -n mesh-operator
NAME                                    READY     STATUS    RESTARTS   AGE
global-sidecar-pilot-796fb554d7-blbml   1/1       Running   0          27s
lazyload-fbcd5dbd9-jvp2s                1/1       Running   0          27s
report-server-855c8cf558-wdqjs          2/2       Running   0          27s
slime-boot-68b6f88b7b-wwqnd             1/1       Running   0          39s
```

```
$ kubectl get po 
NAME                              READY     STATUS    RESTARTS   AGE
global-sidecar-785b58d4b4-fl8j4   1/1       Running   0          68s
```
4. install bookinfo in default namespace
5. enable push on demand
```shell
kubectl label ns default istio-dependency-servicefence=true
```
```shell
kubectl annotate svc productpage istio.dependency.servicefence/status=true
kubectl annotate svc reviews istio.dependency.servicefence/status=true
kubectl annotate svc details istio.dependency.servicefence/status=true
kubectl annotate svc ratings istio.dependency.servicefence/status=true
```
6. make sure SidecarScope is generated
```
$ kubectl get sidecar
NAME          AGE
details       12s
kubernetes    11s
productpage   11s
ratings       11s
reviews       11s
```
```
$ kubectl get sidecar productpage -oyaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  name: productpage
  namespace: default
  ownerReferences:
  - apiVersion: microservice.netease.com/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ServiceFence
    name: productpage
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
7. visit productpage & view accesslog
```
[2021-01-04T07:12:24.101Z] "GET /details/0 HTTP/1.1" 200 - "-" 0 178 36 35 "-" "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:84.0) Gecko/20100101 Firefox/84.0" "83793ccf-545c-4cc2-9a48-82bb70d81a2a" "details:9080" "10.244.3.83:9080" outbound|9080||global-sidecar.default.svc.cluster.local 10.244.1.206:42786 10.97.33.96:9080 10.244.1.206:40108 - -
[2021-01-04T07:12:24.171Z] "GET /reviews/0 HTTP/1.1" 200 - "-" 0 295 33 33 "-" "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:84.0) Gecko/20100101 Firefox/84.0" "010bb2bc-54ab-4809-b3a0-288d60670ded" "reviews:9080" "10.244.3.83:9080" outbound|9080||global-sidecar.default.svc.cluster.local 10.244.1.206:42786 10.99.230.151:9080 10.244.1.206:51512 - -
```
Successful access, the accesslog shows that the backend service is global-sidecar.

8. view productpage's SidecarScope again
```
$ kubectl get sidecar productpage -oyaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  name: productpage
  namespace: default
  ownerReferences:
  - apiVersion: microservice.netease.com/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ServiceFence
    name: productpage
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
reviews and details are automatically added！

9. visit productpage & view accesslog again
```
[2021-01-04T07:35:57.622Z] "GET /details/0 HTTP/1.1" 200 - "-" 0 178 2 2 "-" "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:84.0) Gecko/20100101 Firefox/84.0" "73a6de0b-aac9-422b-af7b-2094bd37094c" "details:9080" "10.244.7.30:9080" outbound|9080||details.default.svc.cluster.local 10.244.1.206:52626 10.97.33.96:9080 10.244.1.206:47396 - default
[2021-01-04T07:35:57.628Z] "GET /reviews/0 HTTP/1.1" 200 - "-" 0 379 134 134 "-" "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:84.0) Gecko/20100101 Firefox/84.0" "edf8c7eb-9558-4d1e-834c-4f238b387fc5" "reviews:9080" "10.244.7.14:9080" outbound|9080||reviews.default.svc.cluster.local 10.244.1.206:42204 10.99.230.151:9080 10.244.1.206:58798 - default
```
Successful access, the backend services are reviews and details.


## Http Plugin Management
// TODO
### Install & Use
// TODO
### Uninstall
// TODO

## Adaptive Ratelimit
### Install & Use
  
**make sure [slime-boot](#install-slime-boot) has been installed.**   

1. Install the limiter module, through slime-boot:
```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: limiter
  namespace: mesh-operator
spec:
  # Default values copied from <project_dir>/helm-charts/slimeboot/values.yaml\
  module:
    - limiter:
        enable: true
        backend: 1
      name: slime-limiter
  //...      
``` 

2. Define smartlimiter resources

```yaml
apiVersion: microservice.netease.com/v1alpha1
kind: SmartLimiter
metadata:
  name: test-svc
  namespace: default
spec:
  descriptors:
  - action:
      quota: "3"      # quota
      fill_interval:
        seconds: 1    # statistical period
    condition: "true" # Turn on limit when the formula in condition is true
```
The above configuration limits 3 requests per second for the test-svc service. After submitting the configuration, the status information and current limit information of the instance under the service will be displayed in `status`, as follows:

```yaml
apiVersion: microservice.netease.com/v1alpha1
kind: SmartLimiter
metadata:
  name: test-svc
  namespace: default
spec:
  descriptors:
  - action:
      quota: "3"
      fill_interval:
        seconds: 1
    condition: "true"
status:
  endPointStatus:
    cpu: "398293"        # cpu(ns)
    cpu_max: "286793"    # map_cpu(ns)
    memory: "68022"      # memory(kb)  
    memory_max: "55236"  # memory_max(kb)
    pod: "1"
  ratelimitStatus:
  - action:
      fill_interval:
        seconds: 1
      quota: "3"
```
####  ratelimit based on monitoring

The monitoring information entry can be configured in `condition`. For example, if the current limit is triggered when the cpu exceeds 300ms, the following configuration can be performed:

```yaml
apiVersion: microservice.netease.com/v1alpha1
kind: SmartLimiter
metadata:
  name: test-svc
  namespace: default
spec:
  descriptors:
  - action:
      quota: "3"
      fill_interval:
        seconds: 1
    condition: "{cpu}>300000" # The unit of cpu is ns. If the cpu value is greater than 300000ns, the limit will be triggered
status:
  endPointStatus:
    cpu: "398293"        
    cpu_max: "286793"    
    memory: "68022"      
    memory_max: "55236"  
    pod: "1"
  ratelimitStatus:
  - action:
      fill_interval:
        seconds: 1
      quota: "3"
```

The formula in the condition will be rendered according to the entry of endPointStatus. If the result of the rendered formula is true, the limit will be triggered.

#### Service ratelimit
Due to the lack of global quota management components, we cannot achieve precise service current limit, but assuming that the load balance is ideal, the number of instance current limit = the number of service current limit/the number of instances. The service current limit of test-svc is 3, then the quota field can be configured to 3/{pod} to achieve service-level current limit. When the service is expanded, you can see the change of the instance current limit in the current limit status bar
。
```yaml
apiVersion: microservice.netease.com/v1alpha1
kind: SmartLimiter
metadata:
  name: test-svc
  namespace: default
spec:
  descriptors:
  - action:
      quota: "3/{pod}" # The calculation will be rendered as 3/3 according to the pod value in endPointStatus
      fill_interval:
        seconds: 1
    condition: "{cpu}>300000" 
    match:
    - exact_match: user
      invert_match: false
      name: Bob
status:
  endPointStatus:
    cpu: "xxxxx"        
    cpu_max: "xxxx"    
    memory: "xxx"       
    memory_max: "xx" 
    pod: "3" # The endpoint of test-svc is expanded to 3
  ratelimitStatus:
  - action:
      fill_interval:
        seconds: 1
      quota: "1" # Obviously, 3/3=1
```
### Uninstall
1. Delete slime-boot configuration
2. Delete smartlimiter configuration
```
for i in $(kubectl get ns);do kubectl delete smartlimiter -n $i --all;done
```
### Example
Take bookinfo as an example.

1. Install istio ( > 1.8 ).
2. Install slime.
```shell
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/ydh926/slime/master/install/easy_install_limiter.sh)"
```
3. Install bookinfo.
4. Create smartlimiter resource for reviews service.
```
$ kubectl apply -f https://raw.githubusercontent.com/ydh926/slime/master/samples/reviews-svc-limiter.yaml  
```
5. make sure resource has been creaded.
```
$ kubectl get smartlimiter reviews -oyaml
apiVersion: microservice.netease.com/v1alpha1
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  descriptors:
  - action:
      quota: "3/{pod}"
      fill_interval:
        seconds: 10
    condition: "true"
```
This configuration indicates that the review service will be limited to three visits in 10s.

6. Confirm whether the corresponding envoyfilter resource is created.
```
$ kubectl get envoyfilter  reviews.default.local-ratelimit -oyaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  creationTimestamp: "2021-01-05T07:28:01Z"
  generation: 1
  name: reviews.default.local-ratelimit
  namespace: default
  ownerReferences:
  - apiVersion: microservice.netease.com/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: SmartLimiter
    name: reviews
    uid: 5eed7271-e8a9-4eda-b5d8-6cd2dd6b3659
  resourceVersion: "59145684"
  selfLink: /apis/networking.istio.io/v1alpha3/namespaces/default/envoyfilters/reviews.default.local-ratelimit
  uid: 04549089-4bf5-4200-98ae-59dd993cda9d
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
                seconds: "10"
              max_tokens: 1
  workloadSelector:
    labels:
      app: reviews
```
Review service can get 3 quotas every 10s, and the service has 3 instances, so each instance can get 1 quota every 10s.

7. visit productpage  
The fourth visit within 10s will trigger limit. View the accesslog of productpage to see the current limiting effect more intuitively:
```
[2021-01-05T07:29:03.986Z] "GET /reviews/0 HTTP/1.1" 429 - "-" 0 18 10 10 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.61 Safari/537.36" "d59c781a-f62c-4e98-9efe-5ace68579654" "reviews:9080" "10.244.8.95:9080" outbound|9080||reviews.default.svc.cluster.local 10.244.1.206:35784 10.99.230.151:9080 10.244.1.206:39864 - default
```                          
