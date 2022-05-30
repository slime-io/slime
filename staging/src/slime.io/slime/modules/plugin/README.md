- [Http Plugin Management](#http-plugin-management)
  - [Install & Use](#install--use)
  - [Inline plugin](#inline-plugin)
  - [PluginManager Example](#pluginmanager-example)
  - [EnvoyPlugin Example](#envoyplugin-example)

[中文](./README_zh.md) 

### Http Plugin Management

#### Install & Use

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



#### Inline plugin

**Note:** Envoy binary needs to support extension plugins
**enable/disable**
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

{{plugin-N}} is the name of the plug-in, and the sort in PluginManager is the execution order of the plug-in. Set the enable field to false to disable the plugin.



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



#### PluginManager Example

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
$ kubectl -n default get envoyfilter reviews-pm -oyaml
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
            name: envoy.http_connection_manager
            subFilter:
              name: envoy.router
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



#### EnvoyPlugin Example

Use the yaml file below to create envoy plugin

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
$ kubectl -n default get envoyfilter reviews-ep -oyaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  creationTimestamp: "2021-08-26T08:13:56Z"
  generation: 1
  name: reviews-ep
  namespace: default
  ownerReferences:
  - apiVersion: microservice.slime.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: EnvoyPlugin
    name: reviews-ep
    uid: fcf9d63b-115f-4a2a-bfc4-40d5ce1bcfee
  resourceVersion: "658067"
  uid: 762768a7-48ae-4939-afa3-f687e0cca826
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

