## 背景

在[http懒加载](./http%E6%87%92%E5%8A%A0%E8%BD%BD.md)中我们提到，兜底路由的替换是http懒加载方案中核心环节之一。

在该环节中，我们需要将listener中的兜底路由allow_any替换成lazyload的兜底路由, 那们我们如何确定哪些listener需要被替换呢？

本文档将介绍lazyload中的一个新特性: 自动端口纳管-autoport

## 介绍

在lazyload最初的场景中, 我们需要在部署指定端口进行懒加载, 但是在实际的场景中, 我们需要对所有的端口进行懒加载。

我们的方案是：

  1. 默认获取集群中的所有http端口, 判断是否是http端口的依据是: 端口的协议是kerbernetes service的端口name必须以http开头

  2. 根据以上获取的端口，自动渲染出兜底的envoyfilter，对于任意端口，我们将生成如下内容，其中{{ $port }}是端口号

    ```yaml
    - applyTo: VIRTUAL_HOST
        match:
        context: SIDECAR_OUTBOUND
        routeConfiguration:
            name: {{ $port }}
            vhost:
            name: allow_any
        patch:
        operation: REMOVE
    - applyTo: ROUTE_CONFIGURATION
        match:
        context: SIDECAR_OUTBOUND
        routeConfiguration:
            name: {{ $port }}
        patch:
        operation: MERGE
        value:
            virtual_hosts:
            - domains:
            - '*'
            name: allow_all
            routes:
            - match:
                prefix: /
                request_headers_to_add:
                - append: false
                header:
                    key: Slime-Orig-Dest
                    value: '%DOWNSTREAM_LOCAL_ADDRESS%'
                - append: false
                header:
                    key: Slime-Source-Ns
                    value: '%ENVIRONMENT(POD_NAMESPACE)%'
                route:
                cluster: outbound|80||global-sidecar.mesh-operator.svc.cluster.local  ## global-sidecar
                timeout: 0s
    ```

通过以上操作，我们将会生成一个envoyfilter，对于所有的http端口，都会将其兜底路由替换为global-sidecar。

**现在还有一个问题，由于我们开启了懒加载，lazyload自动限制了业务服务的可见范围为istio-system下服务以及global-sidecar，那么那些listener是无法下发的，那么我们这些envoyfilter又如何patch至listener呢？**

   3. lazyload的方法是，将上面获取的所有http端口都写入global-sidecar的service上，由于服务的可见范围包括global-sidecar，那么global-sidecar的listener就会被下发，从而使得envoyfilter生效。


## 小结

通过以上的操作，我们可以实现自动端口纳管，从而实现自动兜底路由替换。但是这个方案依旧存在一些问题：

在集群范围内，对于一些grpc或者h2的端口，如果与http端口相同端口号。

如果此时该拥有该h2或者grpc端口的服务也加入了网格，那么这些服务的流量就会被兜底至global-sidecar

但是由于我们目前的global-sidecar不能处理grpc或者h2的流量，所以导致这些服务的流量无法正常处理。

所以对于这种拥有h2或者grpc端口的服务，需要用户关闭懒加载，即在服务svc上加上如下的注解：`slime.io/serviceFenced: "false"`

**我们在grpc懒加载方案中，尝试用envoy代替global-sidecar, 这样就可以直接转发grpc和h2的流量。该方案目前处于实验阶段，还未正式发布，敬请期待。**