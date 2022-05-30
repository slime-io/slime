- [懒加载概述](#懒加载概述)
  - [特点](#特点)
  - [背景](#背景)
  - [思路](#思路)
  - [架构](#架构)
  - [安装和使用](#安装和使用)
  - [特性介绍](#特性介绍)
  - [完整使用样例](#完整使用样例)
  - [E2E测试介绍](#e2e测试介绍)
  - [ServiceFence说明](#servicefence说明)
  - [常见问题](#常见问题)


# 懒加载概述

[English](./README.md)

## 特点

1. 支持1.8+的Istio版本，无侵入性，[版本适配详情](https://github.com/slime-io/slime/issues/145)
2. 可自动对接整个服务网格
3. 兜底转发过程支持Istio所有流量治理能力
4. 兜底逻辑简单，与服务数量无关，无性能问题
5. 支持为服务手动或自动启用懒加载
6. 支持Accesslog和Prometheus等多种动态服务依赖获取方式
7. 支持添加静态服务依赖关系，动静依赖关系结合，功能全面





## 背景

懒加载即按需加载。

没有懒加载时，服务数量过多时，Envoy配置量太大，新上的应用长时间处于Not Ready状态。为应用配置Custom Resource `Sidecar`，并自动的获取服务依赖关系，更新Sidecar可以解决此问题。



## 思路

引入一个服务`global-sidecar`，它是一个兜底应用。它会被集群的Istiod注入sidecar容器。该sidecar容器拥有全量的配置和服务发现信息。兜底路由替换为指向global-sidecar。

引入新的Custom Resource Definition `ServiceFence`。详见[ServiceFence说明](#ServiceFence说明)

最后，将控制逻辑包含到lazyload controller组件中。它会为启用懒加载的服务创建ServiceFence和Sidecar，根据配置获取到服务调用关系，更新ServiceFence和Sidecar。



## 架构

整个模块由Lazyload controller和global-sidecar两部分组成。Lazyload controller无需注入sidecar，global-sidecar则需要注入。

<img src="./media/lazyload-architecture-20211222_zh.png" style="zoom:80%;" />



具体的细节说明可以参见[架构](./lazyload_tutorials_zh.md#%E6%9E%B6%E6%9E%84)



## 安装和使用

1. 根据global-sidecar的部署模式不同，该模块目前分为两种模式：

   - Cluster模式：使用cluster级别的global-sidecar：集群唯一global-sidecar应用

   - Namespace模式：使用namespace级别的global-sidecar：每个使用懒加载的namespace下一个global-sidecar应用

2. 根据服务依赖关系指标来源不同，该模块分为两种模式：

   - Accesslog模式：global-sidecar通过入流量拦截，生成包含服务依赖关系的accesslog
   - Prometheus模式：业务应用在完成访问后，生成metric，上报prometheus。此种模式需要集群对接Prometheus

总的来说，Lazyload模块有4种使用模式，较为推荐Cluster+Accesslog模式。

详见 [安装和使用](./lazyload_tutorials_zh.md#%E5%AE%89%E8%A3%85%E5%92%8C%E4%BD%BF%E7%94%A8)



## 特性介绍

- 可基于Accesslog开启懒加载
- 支持为服务手动或自动启用懒加载
- 支持自定义兜底流量分派
- 支持添加静态服务依赖关系
- 支持自定义服务依赖别名
- 日志输出到本地并轮转

详见 [特性介绍](./lazyload_tutorials_zh.md#%E7%89%B9%E6%80%A7%E4%BB%8B%E7%BB%8D)



## 完整使用样例

详见 [示例: 为bookinfo的productpage服务开启懒加载](./lazyload_tutorials_zh.md#%E7%A4%BA%E4%BE%8B)



## E2E测试介绍

在进行功能开发时，可以通过E2E测试验证模块功能正确性。

详见 [E2E测试教程](https://github.com/slime-io/slime/blob/master/doc/zh/slime_e2e_test_zh.md)



## ServiceFence说明

ServiceFence可以看作是针对某一服务的Sidecar资源，区别是ServiceFence不仅会根据依赖关系生成Sidecar资源，同时会根据VirtualService规则判断服务的真实后端，并自动扩大Fence的范围。

例如，c.default.svc.cluster.local在fence中。此时有一条路由规则的host为c.default.svc.cluster.local，其destinatoin为d.default.svc.cluster.local，那么d服务也会被自动扩充到Fence中。

<img src="./media/ll.png" alt="服务围栏" style="zoom: 67%;" />



## 常见问题

详见 [FAQ](./lazyload_tutorials_zh.md#FAQ)

