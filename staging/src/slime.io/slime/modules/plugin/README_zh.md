- [HTTP插件管理](#http插件管理)
  - [安装和使用](#安装和使用)
  - [PluginManager](#pluginmanager)
    - [内建插件的打开/停用](#内建插件的打开停用)
    - [全局配置](#全局配置)
    - [PluginManager样例](#pluginmanager样例)
  - [EnvoyPlugin](#envoyplugin)
    - [EnvoyPlugin 样例](#envoyplugin-样例)
      - [使用 EnvoyPlugin 配置 RDS typedPerFilterConfig 设置 http filter](#使用-envoyplugin-配置-rds-typedperfilterconfig-设置-http-filter)
      - [使用 EnvoyPlugin 配置非 typedPerFilterConfig 字段设置流量治理规则](#使用-envoyplugin-配置非-typedperfilterconfig-字段设置流量治理规则)

[English](./README.md) 

# HTTP插件管理

## 安装和使用

使用如下配置安装HTTP插件管理模块：

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

[完整样例](./install/samples/plugin/slimeboot_plugin.yaml)

pluginmanager 和 envoyplugin 是平级关系。pluginmanager 作用于 LDS 对象，用于自动生成针对 HTTP 链接管理器（HCM）的路由过滤器（envoy.filters.http.router）的 envoyfilter。envoyplugin 作用于 RDS 对象，用于自动生成针对虚拟主机（config.route.v3.VirtualHost）和路由（config.route.v3.Route）的 envoyfilter。

## PluginManager

### 内建插件的打开/停用

> **注意:** envoy 的二进制需支持扩展插件

按如下格式配置PluginManager，即可打开内建插件：

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: PluginManager
metadata:
  name: reviews-pm
  namespace: default
spec:
  workloadLabels:
    app: reviews
  plugin:
  - enable: true
    name: {{plugin-1}}     # plugin name
  # ...
  - enable: true
    name: {{plugin-N}}
```

其中，`{{plugin-N}}` 为插件名称，PluginManager 中的排序为插件执行顺序。将 enable 字段设置为 false 即可停用插件。

### 全局配置

全局配置对应LDS中的插件配置，按如下格式设置全局配置：

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: PluginManager
metadata:
  name: my-plugin
  namespace: default
spec:
  workloadLabels:
    app: my-app
  plugin:
  - enable: true            # switch
    name: {{plugin-1}}      # plugin name
    inline:
      settings:
        {{plugin_settings}} # plugin settings
  # ...
  - enable: true
    name: {{plugin-N}}
```

### PluginManager样例

按如下格式配置 PluginManager，启用 reviews-ep：

```yaml
apiVersion: microservice.slime.io/v1alpha1
kind: PluginManager
metadata:
  name: reviews-pm
  namespace: default
spec:
  workloadLabels:
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

生成EnvoyFilter如下：

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

EnvoyPlugin 通过配置 envoy RDS api 的 `typedPerFilterConfig` 可以启用并设置指定的 http filter。同时，对在 `typedPerFilterConfig` 之外的流量治理接口，如 `rate_limits(config.route.v3.RateLimit)`、`cors(config.route.v3.CorsPolicy)` 等，EnvoyPlugin 提供了 DirectPatch 模式用于设置这类配置。可按照如下格式配置：

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

其中，`{{api_settings}}` 为 RDS api 对象中非 typedPerFilterConfig 字段的设置。

### EnvoyPlugin 样例

#### 使用 EnvoyPlugin 配置 RDS typedPerFilterConfig 设置 http filter

以配置禁用 `envoy.filters.http.buffer` 为例，按如下格式配置 EnvoyPlugin：

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

生成的envoyfilter如下：

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

#### 使用 EnvoyPlugin 配置非 typedPerFilterConfig 字段设置流量治理规则

以配置 `rate_limits` 和 `cors` 为例，按如下格式配置 EnvoyPlugin：

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

生成的envoyfilter如下：

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
