- [Http Plugin Management](#http-plugin-management)
  - [Install & Use](#install--use)
  - [PluginManager](#pluginmanager)
    - [Enable/Disable Inline Plugin](#enabledisable-inline-plugin)
    - [Global configuration](#global-configuration)
    - [PluginManager Example](#pluginmanager-example)
  - [EnvoyPlugin](#envoyplugin)
    - [EnvoyPlugin Example](#envoyplugin-example)
      - [Use EnvoyPlugin to configure RDS typedPerFilterConfig to set http filters](#use-envoyplugin-to-configure-rds-typedperfilterconfig-to-set-http-filters)
      - [Use EnvoyPlugin to configure non-typedPerFilterConfig fields to set traffic management rules](#use-envoyplugin-to-configure-non-typedperfilterconfig-fields-to-set-traffic-management-rules)

[中文](./README.md) 

# Http Plugin Management

## Install & Use

Use the following configuration to install the HTTP plugin management module:

```yaml
apiVersion: config.netease.com/v1alpha1
kind: SlimeBoot
metadata:
  name: plugin
  namespace: mesh-operator
spec:
  module:
    - name: plugin # custom value
      kind: plugin # should be "plugin"
      enable: true
  image:
    pullPolicy: Always
    repository: docker.io/slimeio/slime-plugin
    tag: {{your_plugin_tag}}
```

[Example](./install/samples/plugin/slimeboot_plugin.yaml)

PluginManager and EnvoyPlugin are at the same level. Pluginmanager acts on the LDS object and is used to automate the generation of envoyfilter for HTTP_Connect_Management(HCM)'s http route filter(envoy.filters.http.router). Envoyplugin acts on the RDS object to automate the generation of envoyfilter for vhosts (config.route.v3.VirtualHost) and routes (config.route.v3.Route).

## PluginManager

### Enable/Disable Inline Plugin

>**Note:** Envoy binary needs to support extension plugins

Configure PluginManager in the following format to open the built-in plugin:

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: PluginManager
metadata:
  name: reviews-pm
  namespace: default
spec:
  workload_labels:
    app: reviews
  plugin:
  - enable: true
    name: {{plugin-1}}     # plugin name
  # ...
  - enable: true
    name: {{plugin-N}}
```

`{{plugin-N}}` is the name of the plugin, and the sort in PluginManager is the execution order of the plug-in. Set the enable field to false to disable the plugin.

### Global configuration

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
  plugin:
  - enable: true          # switch
    name: {{plugin-1}}      # plugin name
    inline:
      settings:
        {{plugin settings}} # plugin settings
  # ...
  - enable: true
    name: {{plugin-N}}
```

### PluginManager Example

Use the yaml file below to create plugin manager, and enable the plugin reviews-ep

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: PluginManager
metadata:
  name: reviews-pm
  namespace: default
spec:
  workload_labels:
    app: reviews
  plugin:
  - enable: true
    name: reviews-ep     # plugin name
    inline:
      settings:
        rate_limits:
        - actions:
          - header_value_match:
              descriptor_value: Service[a.powerful]-User[none]-Gateway[null]-Api[null]-Id[hash:-1414739194]
              headers:
              - invert_match: false
                name: testaaa
                safe_regex_match:
                  google_re2: {}
                  regex: testt
          stage: 0
```

And you will get the envoyfilter

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  creationTimestamp: "2021-08-26T08:20:56Z"
  generation: 1
  name: reviews-pm
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: PluginManager
    name: reviews-pm
    uid: 00a65d02-4025-4d0c-a08a-0a8901cd0fa2
  resourceVersion: "658741"
  uid: 2e8c8a96-fc0d-4e92-9f7a-e3336a53a806
spec:
  configPatches:
  - applyTo: HTTP_FILTER
    match:
      context: SIDECAR_OUTBOUND
      listener:
        filterChain:
          filter:
            name: envoy.filters.network.http_connection_manager
            subFilter:
              name: envoy.filters.http.router
    patch:
      operation: INSERT_BEFORE
      value:
        name: reviews-ep
        typed_config:
          '@type': type.googleapis.com/udpa.type.v1.TypedStruct
          type_url: ""
          value:
            rate_limits:
            - actions:
              - header_value_match:
                  descriptor_value: Service[a.powerful]-User[none]-Gateway[null]-Api[null]-Id[hash:-1414739194]
                  headers:
                  - invert_match: false
                    name: testaaa
                    safe_regex_match:
                      google_re2: {}
                      regex: testt
              stage: 0
  workloadSelector:
    labels:
      app: reviews
```

## EnvoyPlugin

EnvoyPlugin enables and sets the specified http filter by configuring `typedPerFilterConfig` of the envoy RDS api. Also, for traffic management interfaces outside of `typedPerFilterConfig`, such as `rate_limits(config.route.v3.RateLimit)`, `cors(config.route.v3.CorsPolicy)`, EnvoyPlugin provides DirectPatch mode for setting such interfaces. It can be configured in the following format.

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: EnvoyPlugin
metadata:
  name: reviews-ep
  namespace: default
spec:
  route:
    - inbound|http|80/default
  plugin:
  # typedPerFilterConfig
  - enable: true
    name: {{http_filter-1}}     # http filter name
    inline:
      settings:
        {{filter_settings}}     # filter settings
  # ...
  - enable: true
    name: {{http_filter-N}}
  # !typedPerFilterConfig
  - enable: true
    inline:
      directPatch: true
      settings:
        {{api_settings}}
```

`{{api_settings}}` is the setting for the non-typedPerFilterConfig field in the RDS api object.

### EnvoyPlugin Example

#### Use EnvoyPlugin to configure RDS typedPerFilterConfig to set http filters

To disable `envoy.filters.http.buffer`, use the yaml file below to create envoy plugin

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: EnvoyPlugin
metadata:
  name: reviews-ep
  namespace: default
spec:
  route:
    - inbound|http|80/default
  plugins:
  - enable: true
    inline:
      settings:
        disabled: true
    name: envoy.filters.http.buffer
```

And you will get the envoyfilter

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: reviews-ep
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: EnvoyPlugin
    name: reviews-ep
    uid: c2f672bf-588a-4cbe-b0d4-0625ef1320e8
spec:
  configPatches:
  - applyTo: HTTP_ROUTE
    match:
      routeConfiguration:
        vhost:
          name: inbound|http|80
          route:
            name: default
    patch:
      operation: MERGE
      value:
        typedPerFilterConfig:
          envoy.filters.http.buffer:
            '@type': type.googleapis.com/udpa.type.v1.TypedStruct
            type_url: ""
            value:
              disabled: true
```

#### Use EnvoyPlugin to configure non-typedPerFilterConfig fields to set traffic management rules

The envoy RDS api provides some specialized interfaces for configuring traffic management rules in addition to `typedPerFilterConfig`, such as `rate_limits(config.route.v3.RateLimit)`, `cors(config.route.v3. CorsPolicy)`, etc. Such configurations can be set using EnvoyPlugin DirectPatch mode.

Take the configuration of `rate_limits` and `cors` as an example, use the yaml file below to create envoy plugin

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: EnvoyPlugin
metadata:
  name: reviews-ep
  namespace: default
spec:
  workloadSelector:
    labels:
      app: reviews
  route:
    - inbound|http|80/default
  plugins:
  - name: envoy.filters.network.ratelimit
    enable: true
    inline:
      directPatch: true
      settings:
        rate_limits:
        - actions:
          - header_value_match:
              descriptor_value: Service[a.powerful]-User[none]-Gateway[null]-Api[null]-Id[hash:-1414739194]
              headers:
              - invert_match: false
                name: testaaa
                safe_regex_match:
                  google_re2: {}
                  regex: testt
          stage: 0
  - name: envoy.filters.http.cors
    enable: true
    inline:
      directPatch: true
      settings:
        cors:
          allow_origin_string_match:
          - string_match:
              safe_regex_match:
                google_re2: {}
                regex: www.163.com|
```

And you will get the envoyfilter

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: reviews-ep
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: EnvoyPlugin
    name: reviews-ep
    uid: fcf9d63b-115f-4a2a-bfc4-40d5ce1bcfee
spec:
  configPatches:
  - applyTo: HTTP_ROUTE
    match:
      routeConfiguration:
        vhost:
          name: inbound|http|80
          route:
            name: default
    patch:
      operation: MERGE
      value:
        route:
          rate_limits:
          - actions:
            - header_value_match:
                descriptor_value: Service[a.powerful]-User[none]-Gateway[null]-Api[null]-Id[hash:-1414739194]
                headers:
                - invert_match: false
                  name: testaaa
                  safe_regex_match:
                    google_re2: {}
                    regex: testt
            stage: 0
  - applyTo: HTTP_ROUTE
    match:
      routeConfiguration:
        vhost:
          name: inbound|http|80
          route:
            name: default
    patch:
      operation: MERGE
      value:
        route:
          cors:
            allow_origin_string_match:
            - string_match:
                safe_regex_match:
                  google_re2: {}
                  regex: www.163.com|
  workloadSelector:
    labels:
      app: reviews
```
