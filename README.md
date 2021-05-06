# Slime
## Smart ServiceMesh Manager

[中文](https://github.com/slime-io/slime/blob/master/README_ZH.md)  
 
![slime-logo](logo/slime-logo.png)    
 
 [![Go Report Card](https://goreportcard.com/badge/github.com/slime-io/slime)](https://goreportcard.com/report/github.com/slime-io/slime)  

Slime is an intelligent ServiceMesh manager based on istio. Through slime, we can define dynamic service management strategies, so as to achieve the purpose of automatically and conveniently using istio/envoy high-level functions.

**[Configuration Lazy Loading](#configure-lazy-loading):** No need to configure SidecarScope, automatically load configuration on demand.

**[Http Plugin Management](#http-plugin-management):** Use the new CRD pluginmanager/envoyplugin to wrap readability , The poor maintainability of envoyfilter makes plug-in extension more convenient.

**[Adaptive Ratelimit](#adaptive-ratelimit):** It can be automatically combined with adaptive ratelimit strategy based on metrics.

## Architecture
The Slime architecture is mainly divided into three parts:

1. slime-boot，Deploy the operator component of the slime-module, and the slime-module can be deployed quickly and easily through the slime-boot.
2. slime-controller，The core thread of slime-module senses SlimeCRD and converts to IstioCRD.
3. slime-metric，The monitoring acquisition thread of slime-module is used to perceive the service status, and the slime-controller will dynamically adjust the service traffic rules according to the service status.

![slime架构图](media/arch.png)

The user defines the service traffic policy in the CRD spec. At the same time, slime-metric obtains information about the service status from prometheus and records it in the metricStatus of the CRD. After the slime-module controller perceives the service status through metricStatus, it renders the corresponding monitoring items in the service governance policy, calculates the formula in the policy, and finally generates traffic rules.

![limiter治理策略](media/policy.png)

## How to Use Slime

### Install slime-boot
You can easily install and uninstall the slime sub-module with slime-boot. Using the following commands to install slime-boot:
```
kubectl create ns mesh-operator
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/master/install/crds.yaml
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/master/install/slime-boot-install.yaml
```

### Configure lazy loading
#### Install & Use

**make sure [slime-boot](#install-slime-boot) has been installed.**

1. Install the lazyload module and additional components, through slime-boot configuration:
```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  module:
    - fence:
        enable: true
        wormholePort:
        - {{port1}} # svc port
        - {{port2}}
        - ...
      name: slime-fence
      metric:
        prometheus:
          address: #http://prometheus_address
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",report="destination"})by(destination_service)
              type: Group
  component:
    globalSidecar:
      enable: true
      type: namespaced
      namespace:
        - default # app namespace
        - {{you namespace}}
    pilot:
      enable: true
      image:
        repository: docker.io/bcxq/pilot
        tag: preview-1.3.7-v0.0.1
```
2. make sure all components are running
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

3. enable lazyload    
Apply servicefence resource to enable lazyload.
```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: ServiceFence
metadata:
  name: {{your svc}}
  namespace: {{your namespace}}
spec:
  enable: true
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
    - '*/global-sidecar.{your ns}.svc.cluster.local'
  workloadSelector:
    labels:
      app: {{your svc}}
```
#### Other installation options

**Disable global-sidecar**  

In the ServiceMesh with allow_any enabled, the global-sidecar component can be omitted. Use the following configuration:
```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  module:
    - fence:
        enable: true
        wormholePort:
        - {{port1}} 
        - {{port2}}
        - ...
      name: slime-fence
      metric:
        prometheus:
          address: #http://prometheus_address
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",report="destination"})by(destination_service)
              type: Group
```
Not using the global-sidecar component may cause the first call to fail to follow the preset traffic rules.

**Use cluster unique global-sidecar**     

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  module:
    - fence:
        enable: true
        wormholePort:
        - {{port1}} 
        - {{port2}}
        - ...
      name: slime-fence
      metric:
        prometheus:
          address: #http://prometheus_address
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",report="destination"})by(destination_service)
              type: Group
  component:
    globalSidecar:
      enable: true
      type: cluster
      namespace:
        - default 
        - {{you namespace}}
    pilot:
      enable: true
      image:
        repository: docker.io/bcxq/pilot
        tag: preview-1.3.7-v0.0.1      
```

**Use report-server to report the dependency**   

When prometheus is not configured in the cluster, the dependency can be reported through report-server.  

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: lazyload
  namespace: mesh-operator
spec:
  module:
    - fence:
        enable: true
        wormholePort:
        - {{port1}} 
        - {{port2}}
        - ...
      name: slime-fence
      metric:
        prometheus:
          address: #http://prometheus_address
          handlers:
            destination:
              query: |
                sum(istio_requests_total{source_app="$source_app",report="destination"})by(destination_service)
              type: Group
  component:
    globalSidecar:
      enable: true
      type: namespaced
      namespace:
        - default 
        - {{you namespace}}
    pilot:
      enable: true
      image:
        repository: docker.io/bcxq/pilot
        tag: preview-1.3.7-v0.0.1
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
        repository: docker.io/bcxq/mixer
        tag: preview-1.3.7-v0.0.1
      inspectorImage:
        repository: docker.io/bcxq/report-server
        tag: preview-v0.0.1-rc    
```


#### Uninstall

1. Delete corresponding slime-boot configuration.
2. Delete all servicefence configurations.
```shell
for i in $(kubectl get ns);do kubectl delete servicefence -n $i --all;done
```

#### Example

##### Step1: install istio ( > 1.8 )
##### Step2: install slime
1. Download Slime
   ```
   wget https://github.com/slime-io/slime/archive/v0.1.0-alpha.tar.gz
   tar -xvf v0.1.0-alpha.tar.gz
   ```
   
2. Enter the "install" folder
   ```
   cd slime-0.1.0-alpha/install 
   ```
3. Modify installation file of slime-lazyload
   ```shell
   ➜  install cat config/lazyload_install_with_metric.yaml 
   apiVersion: config.netease.com/v1alpha1
   kind: SlimeBoot
   metadata:
     name: lazyload
     namespace: mesh-operator
   spec:
     image:
       pullPolicy: Always
       repository: docker.pkg.github.com/slime-io/slime/slime:v0.1.0
       tag: v0.1.0
     module:
       - name: lazyload
         fence:
           enable: true
           wormholePort: # replace to your application svc ports
             - "9080"
         metric:
           prometheus:
             address: #http://prometheus_address
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
           - default # 替换为bookinfo安装的ns
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
           repository: docker.io/bcxq/pilot
           tag: preview-1.3.7-v0.0.1           
   ```
   Replace '#http://prometheus_address' with the service address of Istio's prometheus.    
4. Execute the installation script
   ```shell
   cd sample/lazyload
   ./easy_install_lazyload.sh
   ```
5. Make sure all components are running
```
$ kubectl get po -n mesh-operator
NAME                                    READY     STATUS    RESTARTS   AGE
global-sidecar-pilot-796fb554d7-blbml   1/1       Running   0          27s
lazyload-fbcd5dbd9-jvp2s                1/1       Running   0          27s
slime-boot-68b6f88b7b-wwqnd             1/1       Running   0          39s
```
```
$ kubectl get po 
NAME                              READY     STATUS    RESTARTS   AGE
global-sidecar-785b58d4b4-fl8j4   1/1       Running   0          68s
```
##### Step3: Try it with bookinfo
1. install bookinfo in default namespace
2. enable push on demand for productpage
```shell
kubectl apply -f sample/lazyload/productpage-servicefence.yaml
```
3. make sure SidecarScope is generated
```
$ kubectl get sidecar
NAME          AGE
productpage   11s
```
```
$ kubectl get sidecar productpage -o yaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  name: productpage
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
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
4. visit productpage & view accesslog
```
[2021-01-04T07:12:24.101Z] "GET /details/0 HTTP/1.1" 200 - "-" 0 178 36 35 "-" "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:84.0) Gecko/20100101 Firefox/84.0" "83793ccf-545c-4cc2-9a48-82bb70d81a2a" "details:9080" "10.244.3.83:9080" outbound|9080||global-sidecar.default.svc.cluster.local 10.244.1.206:42786 10.97.33.96:9080 10.244.1.206:40108 - -
[2021-01-04T07:12:24.171Z] "GET /reviews/0 HTTP/1.1" 200 - "-" 0 295 33 33 "-" "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:84.0) Gecko/20100101 Firefox/84.0" "010bb2bc-54ab-4809-b3a0-288d60670ded" "reviews:9080" "10.244.3.83:9080" outbound|9080||global-sidecar.default.svc.cluster.local 10.244.1.206:42786 10.99.230.151:9080 10.244.1.206:51512 - -
```
Successful access, the accesslog shows that the backend service is global-sidecar.

5. view productpage's SidecarScope again (You may need to wait about 30s, because prometheus is not updated in real time)
```
$ kubectl get sidecar productpage -oyaml
apiVersion: networking.istio.io/v1beta1
kind: Sidecar
metadata:
  name: productpage
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
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

6. visit productpage & view accesslog again
```
[2021-01-04T07:35:57.622Z] "GET /details/0 HTTP/1.1" 200 - "-" 0 178 2 2 "-" "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:84.0) Gecko/20100101 Firefox/84.0" "73a6de0b-aac9-422b-af7b-2094bd37094c" "details:9080" "10.244.7.30:9080" outbound|9080||details.default.svc.cluster.local 10.244.1.206:52626 10.97.33.96:9080 10.244.1.206:47396 - default
[2021-01-04T07:35:57.628Z] "GET /reviews/0 HTTP/1.1" 200 - "-" 0 379 134 134 "-" "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:84.0) Gecko/20100101 Firefox/84.0" "edf8c7eb-9558-4d1e-834c-4f238b387fc5" "reviews:9080" "10.244.7.14:9080" outbound|9080||reviews.default.svc.cluster.local 10.244.1.206:42204 10.99.230.151:9080 10.244.1.206:58798 - default
```
Successful access, the backend services are reviews and details.

### Http Plugin Management
#### Install & Use
Use the following configuration to install the HTTP plugin management module:
```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: example-slimeboot
  namespace: mesh-operator
spec:
  module:
    - plugin:
        enable: true
        local:
          mount: /wasm/test # wasm文件夹，需挂载在sidecar中    
  image:
    pullPolicy: Always
    repository: docker.pkg.github.com/slime-io/slime/slime:v0.1.0
    tag: v0.1.0
```
#### inline plugin
**Note:** Envoy binary needs to support extension plugins
**enable/disable**
Configure PluginManager in the following format to open the built-in plugin:
```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: PluginManager
metadata:
  name: my-plugin
  namespace: default
spec:
  workload_labels:
    app: my-app
  plugins:
  - enable: true          # switch
    name: {plugin-1}
  # ...
  - enable: true
    name: {plugin-N}
```
{plugin-N} is the name of the plug-in, and the sort in PluginManager is the execution order of the plug-in.
Set the enable field to false to disable the plugin.
**Global configuration**

The global configuration corresponds to the plug-in configuration in LDS. Set the global configuration in the following format:
```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: PluginManager
metadata:
  name: my-plugin
  namespace: default
spec:
  workload_labels:
    app: my-app
  plugins:
  - enable: true          # 插件开关
    name: {plugin-1}      # 插件名称
    inline:
      settings:
        {plugin settings} # 插件配置
  # ...
  - enable: true
    name: {plugin-N}
```


**Host/route level configuration**

Configure EnvoyPlugin in the following format:
```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: EnvoyPlugin
metadata:
  name: project1-abc
  namespace: gateway-system
spec:
  workload_labels:
    app: my-app
  host:                          # Effective range(host level)              
  - jmeter.com
  - istio.com
  - 989.mock.qa.netease.com
  - demo.test.com
  - netease.com
  route:                         # Effective range(route level), The route field must correspond to the name in VirtualService
  - abc
  plugins:
  - name: com.netease.supercache # plugin name
    settings:                    # plugin settings
      cache_ttls:
        LocalHttpCache:
          default: 60000
      enable_rpx:
        headers:
        - name: :status
          regex_match: 200|
      key_maker:
        exclude_host: false
        ignore_case: true
      low_level_fill: true
```

### Adaptive Ratelimit
#### Install & Use

**make sure [slime-boot](#install-slime-boot) has been installed.**   

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
    repository: docker.pkg.github.com/slime-io/slime/slime:v0.1.0
    tag: v0.1.0
  module:
    - limiter:
        enable: true
      metric:
        prometheus:
          address: #http://prometheus_address
          handlers:
            cpu.sum:
              query: |
                sum(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""})
            cpu.max:
              query: |
                max(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""})
        k8s:
          handlers:
            - pod # inline
      name: limiter    
```
In the example, we configure prometheus as the monitoring source, and "prometheus handlers" defines the attributes that we want to obtain from monitoring. These attributes can be used as parameters in the traffic rules to achieve the purpose of adaptive ratelimit. Refer to [Adaptive current limiting based on monitoring] (#Adaptive current limiting based on monitoring).
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
            seconds: 1
          quota: "10"
        condition: "true"
```
The above configuration limits 10 requests per second for the v1 version of the reviews service. After submitting the configuration, the status information and ratelimit information of the instance under the service will be displayed in `status`, as follows:

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  sets:
    v1: # reviews的v1版本
      descriptor:
      - action:
          fill_interval:
            seconds: 1
          quota: "10"
        condition: "true"
status:
  metricStatus:
  # ...
  ratelimitStatus:
    v1:
      descriptor:
      - action:
          fill_interval:
            seconds: 1
          quota: "10"
```
####  Adaptive ratelimit based on metrics
The metrics information entry can be configured in `condition`. For example, if the current limit is triggered when the cpu exceeds 300ms, the following configuration can be performed:

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
            seconds: 1
          quota: "5"
        condition: '{{.v2.cpu.sum}}>10000' 
status:
  metricStatus:
    '_base.cpu.max': "11279.791407871"
    '_base.cpu.sum': "31827.916205633"
    '_base.pod': "3"
    v1.cpu.max: "9328.098703551"
    v1.cpu.sum: "9328.098703551"
    v1.pod: "1"
    v2.cpu.max: "11220.026094211"
    v2.cpu.sum: "11220.026094211"
    v2.pod: "1"
    v3.cpu.max: "11279.791407871"
    v3.cpu.sum: "11279.791407871"
    v3.pod: "1"
  ratelimitStatus:
    v2:
      descriptor:
      - action:
          fill_interval:
            seconds: 1
          quota: "5"
```
The formula in the condition will be rendered according to the entry of endPointStatus. If the result of the rendered formula is true, the limit will be triggered.

#### Service ratelimit
Due to the lack of global quota management components, we cannot achieve precise service ratelimit, but assuming that the load balance is ideal, 
(instance quota) = (service quota)/(the number of instances). The service quota of test-svc is 3, then the quota field can be configured to 3/{pod} to achieve service-level ratelimit. When the service is expanded, you can see the change of the instance quota in status.
```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: SmartLimiter
metadata:
  name: reviews
  namespace: default
spec:
  sets:
    "_base":
      descriptor:
      - action:
          fill_interval:
            seconds: 1
          quota: "3/{{._base.pod}}"
status:
  metricStatus:
    '_base.cpu.max': "11279.791407871"
    '_base.cpu.sum': "31827.916205633"
    '_base.pod': "3"
    v1.cpu.max: "9328.098703551"
    v1.cpu.sum: "9328.098703551"
    v1.pod: "1"
    v2.cpu.max: "11220.026094211"
    v2.cpu.sum: "11220.026094211"
    v2.pod: "1"
    v3.cpu.max: "11279.791407871"
    v3.cpu.sum: "11279.791407871"
    v3.pod: "1"
  ratelimitStatus:
    _base:
      descriptor:
      - action:
          fill_interval:
            seconds: 1
          quota: "1" # For each instance, the current limit quota is 3/3=1.
```
#### Uninstall
1. Delete slime-boot configuration
2. Delete smartlimiter configuration
```
for i in $(kubectl get ns);do kubectl delete smartlimiter -n $i --all;done
```

#### Example
##### Step1: install istio ( > 1.8 )
##### Step2: install slime
1. Download Slime
   ```
   wget https://github.com/slime-io/slime/archive/v0.1.0-alpha.tar.gz
   tar -xvf v0.1.0-alpha.tar.gz
   ```
2. Enter the "install" folder
   ```
   cd slime-0.1.0-alpha/install 
   ```
3. Modify installation file of slime-limiter
   ```shell
   apiVersion: config.netease.com/v1alpha1
   kind: SlimeBoot
   metadata:
     name: smartlimiter
     namespace: mesh-operator
   spec:
     image:
       pullPolicy: Always
       repository: docker.pkg.github.com/slime-io/slime/slime:v0.1.0
       tag: v0.1.0
     module:
       - limiter:
           enable: true
           backend: 1
         metric:
           prometheus:
             address: #http://prometheus_address
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
   Replace '#http://prometheus_address' with the service address of Istio's prometheus.    
##### Step3: Try it with bookinfo 

1. Install bookinfo.
2. Create smartlimiter resource for reviews service.
```shell
kubectl apply -f sample/limiter/reviews.yaml
```
3. make sure resource has been created.
```
$ kubectl get smartlimiter reviews -oyaml
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
        condition: '{{._base.rt99}}>10'
status:
  metricStatus:
    _base.cpu.max: "222.692507384"
    _base.cpu.sum: "590.506922302"
    _base.pod: "19"
    v1.cpu.max: "199.51872224"
    v1.cpu.sum: "199.51872224"
    v1.pod: "3"
    v2.cpu.max: "222.692507384"
    v2.cpu.sum: "222.692507384"
    v2.pod: "11"
    v3.cpu.max: "168.295692678"
    v3.cpu.sum: "168.295692678"
    v3.pod: "5"

```
This configuration indicates that when 99% of the request time is greater than 10ms, the rate-limit will be triggered. 
reviews service can get 3 quotas every 60s. 

4. Confirm whether the corresponding envoyfilter resource is created.
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
  - apiVersion: microservice.slime.io/v1alpha1
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
                seconds: "60"
              max_tokens: 1
  workloadSelector:
    labels:
      app: reviews
```
Review service can get 3 quotas every 60s, and the service has 3 instances, so each instance can get 1 quota every 60s.

5. visit productpage  
The fourth visit within 10s will trigger limit. View the accesslog of productpage to see the ratelimit effect more intuitively:
```
[2021-01-05T07:29:03.986Z] "GET /reviews/0 HTTP/1.1" 429 - "-" 0 18 10 10 "-" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.61 Safari/537.36" "d59c781a-f62c-4e98-9efe-5ace68579654" "reviews:9080" "10.244.8.95:9080" outbound|9080||reviews.default.svc.cluster.local 10.244.1.206:35784 10.99.230.151:9080 10.244.1.206:39864 - default
```
