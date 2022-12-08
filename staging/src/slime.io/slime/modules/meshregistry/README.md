

# Introduction

The role of the `meshregistry` module is approximately `serviceregistry`, which docks a variety of service registries, converts different service data into unified `ServiceEntry` data (istio service model), and then provides it to the downstream as a service by means of a specified protocol.

This module can be considered as a continuation of the istio `galley` component, with some differences.

* No longer proxies resources other than service data for single responsibility reasons (`galley` supports fetching various configuration resources in k8s and forwarding them to downstream)

* Following the evolution of istio, the supported protocols are switched from `MCP` to `MCP-over-xDS` (this is achieved by [istio-mcp](https://github.com/slime-io/istio-mcp), the library also supports HTTP protocols, etc.)

* Some enhancements have been made based on the experience and needs of the ground practice, some of which may require downstream (istio side) corresponding modifications, but are not necessary

# Functional features

## Docking service registry

The currently supported service registries are.

* zk (dubbo)

* eureka

* nacos

* k8s



## Supported protocols

* `MCP-over-xDS`

* [istio-mcp](https://github.com/slime-io/istio-mcp) Other protocols supported



## Incremental push

This feature is supported by [istio-mcp](https://github.com/slime-io/istio-mcp) and requires client-server dual-end support. Currently, since it is not yet in the community istio, you need to turn off this feature if you dock to the community istio. If you expect to use it, you need to modify istio to replace the community `Adsc` with this library.



> There are plans to retrofit istio-mcp incremental push to delta xDS implementation and then move forward with community adsc support for delta xDS. You can follow the progress of this library



## dubbo support

### dubbo `Sidecar` generation

Optionally, you can enable the dubbo `Sidecar` generation feature, which will analyze the dubbo application's interface dependencies based on the dubbo consumers' information in zk and then generate standard istio `Sidecar` resources, which can greatly reduce the amount of configuration sent down to the data surface

# Use

As a slime module, the flow of use is roughly the same: 1.

1. enable the module and be managed by slime (slime-boot method or bundle method)

2. prepare the module config to configure the module behavior as necessary

## bundle deployment method

1. add the module to the bundle, package it, and create an image

2. this module supports a small number of behavior settings in the env way, if necessary, you can add in the bundle's deployment material (deployment) (optional)

3. in the bundle's configuration configmap to add this module section, similar to the following.

   ```yaml
   apiVersion: v1
   data:
     cfg: |
       bundle:
         modules:
         - name: meshregistry
           kind: meshregistry
       enable: true
     cfg_meshregistry: |
       name: meshregistry
       kind: meshregistry
       enable: true
       mode: BundleItem
       general:
         LEGACY:
           MeshConfigFile: ""
           RevCrds: ""
           Mcp:
             EnableIncPush: false
           K8SSource:
             Enabled: false
           EurekaSource:
             Enabled: true
             Address:
             - "http://eureka.myeureka.com/eureka"
             RefreshPeriod: 15s
             SvcPort: 80
           ZookeeperSource:
             Enabled: true
             RefreshPeriod: 30s
             WaitTime: 10s
             # EnableDubboSidecar: false
             ZookeeperSource: Enabled: true
             - zookeeper.myzk.cluster.local:2181
   ```

   > See the bundle deployment mode introduction for other configurations
   >
   > See `pkg/bootstrap/args.go` for configuration details

