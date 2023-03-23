- [Smart ServiceMesh Manager](#smart-servicemesh-manager)
  - [Why Slime](#why-slime)
  - [Architecture](#architecture)
  - [Tutorials](#tutorials)
  - [Community](#community)
  - [License](#license)

# Smart ServiceMesh Manager

[中文](./README.md)

![slime-logo](media/slime_logo.png)    

 [![Go Report Card](https://goreportcard.com/badge/github.com/slime-io/slime)](https://goreportcard.com/report/github.com/slime-io/slime) [![License](https://img.shields.io/badge/License-Apache%202.0-green.svg)](https://github.com/slime-io/slime/blob/master/LICENSE) ![GitHub release (latest by date)](https://img.shields.io/github/v/release/slime-io/slime?color=green)

Slime is an intelligent ServiceMesh manager based on istio. Through slime, we can define dynamic service management strategies, so as to achieve the purpose of automatically and conveniently using istio/envoy high-level functions.


## Why Slime

As a new generation of microservice architecture, Service mesh realizes the decoupling of business logic and microservice governance logic, and reduces the development and operation and maintenance costs of microservices. However, in the process of helping business teams to use Service mesh and implement it in production, we found that there are still many problems with the existing Service mesh.

- Some functions are missing or the threshold for use is too high, resulting in unsuccessful business access.
- many stability risks in large-scale business cluster scenarios.
- Difficulties in management and operation and maintenance of the service mesh by administrators: the service mesh base framework needs to be modified to solve the problem. This will change the original logic of the base framework, which cannot be merged into the community version and creates a lot of difficulties for developers to maintain the service mesh in the long term.

For this reason, we have developed a number of Service mesh peripheral modules to solve these problems and ensure that the enterprise business running on top of the Service mesh can run smoothly, and designed extension mechanisms do not need to invade the native code of the framework. In order to give back to the community, we systematically compiled core modules to solve common problems and open source them, which led to the Slime project.

The project is based on the k8s-operator implementation, **which seamlessly interfaces with Istio without any customization**.

Slime core capabilities include intelligent traffic management, intelligent operation and maintenance management, intelligent extension management.

- **Intelligent Traffic Management**: Upgrades Service mesh traffic governance capabilities through feature content in business traffic to provide more granular and timely governance capabilities for business --
  - [Adaptive Rate Limiting](./staging/src/slime.io/slime/modules/limiter): realizes local rate limiting, and at the same time can automatically adjust the rate limiting policy in conjunction with monitoring information, filling the shortcomings of the traditional service mesh flow limiting function
  - Intelligent degradation
  - Traffic mark

- **Intelligent O&M Management**: Combining the components and business features under the Service mesh architecture to provide more accurate and visualized O&M capabilities and performance stability enhancement --
  - [configure lazy loading](./staging/src/slime.io/slime/modules/lazyload): no need to configure SidecarScope, automatically load configuration and service discovery information on demand, solving the problem of full volume push. The source of the service call relationship supports Prometheus or Accesslog
  - [mesh(service) repository](./staging/src/slime.io/slime/modules/meshregistry): helps istio to quickly integrate various service registries
  - File distribution management (filemanager, to be provided later)
  - Command line interaction [i9s](https://github.com/slime-io/i9s)
  - Patrol (patrol)
  - Troubleshooting tools (tracetio)

- **Intelligent plugin management**: for the lack of efficient plugin management tools for the service mesh, to provide bulk plugin management capabilities to simplify the difficulty of managing plug-ins on the data surface of the service mesh
  - [Http plugin management](./staging/src/slime.io/slime/modules/plugin): using the new CRD pluginmanager/envoyplugin wraps the poor readability and maintainability envoyfilter, making it easier to extend the plugin.

  
## Architecture

The Slime architecture is mainly divided into three parts:

1. slime-boot，the operator component which can deploy Slime (slime-modules and slime-framework).
2. slime-modules，core processes of Slime, watch Slime CRD and convert to Istio CRD, and process other built-in logic.
3. slime-framework, as a base, provide generic base capabilities for modules.

![slime架构图](media/slime-arch-v2.png)

Slime supports aggregated packaging, allowing any module to be aggregated into a single image. So Slime can be deployed as a single deployment, avoiding too many components.

## Tutorials

[Slime Website](https://slime-io.github.io/)

[Slime Image Info](https://github.com/slime-io/slime/wiki/Slime-Project-Tag-and-Image-Tag-Mapping-Table)

[Slime-boot Install](./doc/en/slime-boot.md)

Slime-module

- [Lazyload Usage](./staging/src/slime.io/slime/modules/lazyload/README_EN.md)
- [PluginManager Usage](./staging/src/slime.io/slime/modules/plugin/README_EN.md)
- [SmartLimiter Usage](./staging/src/slime.io/slime/modules/limiter/README_EN.md)
- [MeshRegistry Usage](./staging/src/slime.io/slime/modules/meshregistry/README_EN.md)



[E2E测试教程](./doc/zh/slime_e2e_test_zh.md)




## Community

- Wechat Group: Please contact Wechat ID: `yonka_hust` to join the group
- QQ Group: 971298863
- Slack: [https://slimeslime-io.slack.com/invite](https://join.slack.com/t/slimeslime-io/shared_invite/zt-u3nyjxww-vpwuY9856i8iVlZsCPtKpg)
- email: slimedotio@gmail.com
- You'll find many other useful documents on our official web [Slime-Home](https://slime-io.github.io/)

## License

[Apache-2.0](https://choosealicense.com/licenses/apache-2.0/)

