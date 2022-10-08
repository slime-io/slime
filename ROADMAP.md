- [ROADMAP](#roadmap)
  - [Slime v0.4.0 (Released)](#slime-v040-released)
  - [Slime v0.5.0 (Release in progress)](#slime-v050-release-in-progress)
  - [Slime v0.6.0 (In Planning)](#slime-v060-in-planning)
  - [Slime v0.7.0 (In Planning)](#slime-v070-in-planning)
- [路线图](#路线图)
  - [Slime v0.4.0 （已发布）](#slime-v040-已发布)
  - [Slime v0.5.0（发布中）](#slime-v050发布中)
  - [Slime v0.6.0（规划中）](#slime-v060规划中)
  - [Slime v0.7.0（规划中）](#slime-v070规划中)



# ROADMAP

## Slime v0.4.0 (Released)

**Traffic Management**

- [SmartLimiter] Support for adding limit rules to inbound traffic

**Operation Management**

- [Lazyload] Dynamic service dependency persistence

- [Lazyload] Prometheus Metric mode supports Istio version 1.12+

**Extension Management**

- [Plugin] PluginManager supports port matching

**Engineering**

- Support for adding custom HTTP interfaces to modules

- Support for rapid generation of new Slime Blank modules

- Support for generating multiple architecture mirrors



## Slime v0.5.0 (Release in progress)

**Traffic Management**

- [SmartLimiter] Release of Service Mesh Limit standard API
- [SmartLimiter] Supports local traffic limitation for gateway scenarios
- [SmartLimiter] Support for multi-service registration centers

**Operation Management**

- [Lazyload] Support for auto-regulation of all services
- [Lazyload] Support for auto-regulation of all ports
- [Lazyload] Supports multi-cluster with same network scenarios
- [i9s] Support for the Istio debug API view
- [i9s] Support for the Envoy debug API view

**Engineering**

- More user-friendly version management, aggregation of Slime modules code to the main project
- Support for registering HTTP redirection interfaces



## Slime v0.6.0 (In Planning)

**Traffic Management**

- [SmartDowngrade] Release of Service Mesh downgrade standard API
- [SmartDowngrade] Release of New smart downgrade module

**Operation Management**

- [Lazyload] Release of Service Mesh accurate pushing standard API
- [Lazyload] Support for multi-service registration centers

**Extension Management**

- [Plugin] Support for service level plugin distribution

**Engineering**

- Compatiable with Kubernetes 1.22+



## Slime v0.7.0 (In Planning)

**Traffic Management**

- [SmartMeltdown] Release of Service Mesh meltdown standard API
- [SmartMeltdown] Release of New smart meltdown module





# 路线图

## Slime v0.4.0 （已发布）

**流量管理**

- 【智能限流】支持对入口流量添加限流规则

**运维管理**

- 【配置懒加载】实现动态服务依赖关系持久化

- 【配置懒加载】Prometheus Metric 模式支持 Istio 1.12+ 版本

**扩展管理**

- 【插件管理】PluginManager支持端口匹配

**工程**

- 支持给模块添加自定义 HTTP 接口

- 支持快速生成 Slime 空白新模块

- 支持打多架构镜像



## Slime v0.5.0（发布中）

**流量管理**

- 【智能限流】发布服务网格限流标准 API
- 【智能限流】支持网关场景本地限流
- 【智能限流】支持多服务注册中心

**运维管理**

- 【配置懒加载】支持自动纳管所有服务
- 【配置懒加载】支持自动纳管所有端口
- 【配置懒加载】支持同网络多集群场景
- 【i9s】支持 Istio Debug API 视图
- 【i9s】支持 Envoy Debug API 视图

**工程**

- 更友好的版本管理，Slime 子模块代码聚合至主项目
- 支持注册 HTTP 重定向接口



## Slime v0.6.0（规划中）

**流量管理**

- 【智能降级】发布服务网格降级标准 API
- 【智能降级】发布智能降级新模块

**运维管理**

- 【配置懒加载】发布服务网格配置精准推送标准 API
- 【配置懒加载】支持多服务注册中心

**扩展管理**

- 【插件管理】支持服务级别插件下发

**工程**

- 适配 Kubernetes 1.22+



## Slime v0.7.0（规划中）

**流量管理**

- 【智能熔断】发布服务网格熔断标准 API
- 【智能熔断】发布智能熔断新模块



