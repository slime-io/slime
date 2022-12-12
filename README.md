- [智能网格管理器](#智能网格管理器)
  - [为什么选择Slime](#为什么选择slime)
  - [架构](#架构)
  - [教程](#教程)
  - [交流](#交流)
  - [证书](#证书)

# 智能网格管理器

[English](./README_EN.md)

![slime-logo](media/slime_logo.png)

 [![Go Report Card](https://goreportcard.com/badge/github.com/slime-io/slime)](https://goreportcard.com/report/github.com/slime-io/slime) [![License](https://img.shields.io/badge/License-Apache%202.0-green.svg)](https://github.com/slime-io/slime/blob/master/LICENSE) ![GitHub release (latest by date)](https://img.shields.io/github/v/release/slime-io/slime?color=green)

---
Slime是基于Istio的智能网格管理器。通过Slime，我们可以定义动态的服务治理策略，从而达到自动便捷使用Istio和Envoy高阶功能的目的。





## 为什么选择Slime

服务网格作为新一代微服务架构，采用 Istio+Envoy ，实现了业务逻辑和微服务治理逻辑的物理解耦，降低微服务框架的开发与运维成本。

Istio 可以实现版本分流、灰度发布、负载均衡等功能，但是在本地限流，黑白名单，降级等微服务治理的高阶特性存在缺陷。起初 Istio 给出的解决方案是 Mixer，将这些原本属于数据面的功能上升到 Mixer Adapter 中。这样做虽然解决了功能扩展的问题，但集中式的架构遭到了不少关注者对其性能的质疑。最终，Istio 在新版本中自断其臂，弃用了 Mixer，这就使得高阶功能的扩展成为目前版本的一块空白。

另一方面 Istio 配置是全量推送的。这就意味着在大规模的网格场景下需推送海量配置。为了减少推送配置量，用户不得不事先搞清楚服务间的依赖关系，配置 SidecarScope做配置隔离，而这无疑增加了运维人员的心智负担，易用性和性能成为不可兼得的鱼和熊掌。

针对 Istio 目前的一些弊端，我们推出了Slime项目。该项目是基于 k8s-operator 实现的，作为 Istio 的 CRD 管理器，**可以无缝对接 Istio，无需任何的定制化改造**。

Slime 内部采用了模块化的架构。目前包含了三个非常实用的子模块。

[配置懒加载](./staging/src/slime.io/slime/modules/lazyload)：无须配置SidecarScope，自动按需加载配置和服务发现信息 ，解决了全量推送的问题。服务调用关系的来源支持Prometheus或者Accesslog。

[Http插件管理](./staging/src/slime.io/slime/modules/plugin)：使用新的的CRD pluginmanager/envoyplugin包装了可读性及可维护性差的envoyfilter，使得插件扩展更为便捷。

[自适应限流](./staging/src/slime.io/slime/modules/limiter)：实现了本地限流，同时可以结合监控信息自动调整限流策略，填补了 Istio 限流功能的短板。

[网格（服务）仓库](./staging/src/slime.io/slime/modules/meshregistry)：帮助istio快速集成各种服务注册中心。

后续我们会开源更多的功能模块。


## 架构
Slime架构主要分为三大块：

1. slime-boot，部署Slime（slime-modules和slime-framework）的Operator组件。
2. slime-modules，Slime的核心线程，感知SlimeCRD并转换为IstioCRD，并触发内置的其他逻辑。
3. slime-framework，作为底座，为modules提供通用的基础能力。

其架构图如下：

![slime架构图](media/slime-arch-v2.png)

Slime支持聚合打包，可以将任意模块聚合成一个镜像。所以，Slime可以以一个Deployment形式部署，避免组件过多。



## 教程

[slime网站](https://slime-io.github.io/)

[Slime镜像信息](https://github.com/slime-io/slime/wiki/Slime-Project-Tag-and-Image-Tag-Mapping-Table)

[Slime-boot安装](./doc/zh/slime-boot.md)

Slime-module
- [懒加载使用](./staging/src/slime.io/slime/modules/lazyload/README.md)
- [插件管理使用](./staging/src/slime.io/slime/modules/plugin/README.md)
- [自适应限流使用](./staging/src/slime.io/slime/modules/limiter/README.md)
- [网格（服务）仓库](./staging/src/slime.io/slime/modules/meshregistry)：帮助istio快速集成各种服务注册中心。


[E2E测试教程](./doc/zh/slime_e2e_test_zh.md)



## 交流

- 微信群： 请添加微信号 `yonka_hust` 进群

- QQ群: 971298863
- Slack: [https://slimeslime-io.slack.com/invite](https://join.slack.com/t/slimeslime-io/shared_invite/zt-u3nyjxww-vpwuY9856i8iVlZsCPtKpg)
- 邮件：slimedotio@gmail.com
- 其他有用的信息可以查阅我们的官网 [Slime-Home](https://slime-io.github.io/)




## 证书

[Apache-2.0](https://choosealicense.com/licenses/apache-2.0/)
