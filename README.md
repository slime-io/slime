- [Smart ServiceMesh Manager](#smart-servicemesh-manager)
  - [Why Slime](#why-slime)
  - [Architecture](#architecture)
  - [Tutorials](#tutorials)
  - [Community](#community)
  - [License](#license)

# Smart ServiceMesh Manager

[中文](./README_ZH.md)  

![slime-logo](logo/slime-logo.png)    

 [![Go Report Card](https://goreportcard.com/badge/github.com/slime-io/slime)](https://goreportcard.com/report/github.com/slime-io/slime)  

Slime is an intelligent ServiceMesh manager based on istio. Through slime, we can define dynamic service management strategies, so as to achieve the purpose of automatically and conveniently using istio/envoy high-level functions.





## Why Slime

As new generation architecture of micro-service, service mesh uses Istio+Envoy to achieve the decouple of business logic and micro-service control logic. Thus it can decrease the budget of devlopment and operation.

Istio has many functions, such as multi-version control, grayscale release, load balancer. However, it is not perfect in high-level features of microservice governance, like local rate limit, black and white list, downgrade. Mixer is born to solve these problem by aggregating these data plane functions to Mixer Adapter. Although this solves the problem of function expansion, the centralized architecture has been questioned by many followers about its performance. Then Istio abandoned Mixer in the new version. This makes the expansion of high-level functions a piece of void in the current version.

Another problem is Pilot config is full push. This means massive configurations need to be pushed in large-scale grid scenarios. Users have to figure out the dependencies between services and create SidecarScope in advance. This undoubtedly increases the burden on operation and maintenance personnel.

In order to solving the current shortcomings of Istio, we make Slime project. It is based on Kubernetes operator. As a CRD controller of Istio, Slime can **seamlessly using with Istio without any customization**.

Slime adopts a modular architecture inside. It contains three useful modules now.

[Configuration Lazy Loading](https://github.com/slime-io/lazyload): No need to configure SidecarScope, automatically load configuration on demand, solving full push problem.

[Http Plugin Management](https://github.com/slime-io/plugin): Use the new CRD pluginmanager/envoyplugin to wrap readability , The poor maintainability of envoyfilter makes plug-in extension more convenient.

[Adaptive Ratelimit](https://github.com/slime-io/limiter): It can be automatically combined with adaptive ratelimit strategy based on metrics, solving rate limit problem.





## Architecture

The Slime architecture is mainly divided into three parts:

1. slime-boot，Deploy the operator component of the slime-module, and the slime-module can be deployed quickly and easily through the slime-boot.
2. slime-controller，The core thread of slime-module senses SlimeCRD and converts to IstioCRD.
3. slime-metric，The monitoring acquisition thread of slime-module is used to perceive the service status, and the slime-controller will dynamically adjust the service traffic rules according to the service status.

![slime架构图](media/arch.png)

The user defines the service traffic policy in the CRD spec. At the same time, slime-metric obtains information about the service status from prometheus and records it in the metricStatus of the CRD. After the slime-module controller perceives the service status through metricStatus, it renders the corresponding monitoring items in the service governance policy, calculates the formula in the policy, and finally generates traffic rules.

![limiter治理策略](media/policy.png)





## Tutorials

[Slime Image Info](https://github.com/slime-io/slime/wiki/Slime-Project-Tag-and-Image-Tag-Mapping-Table)

[Slime-boot Install](./doc/en/slime-boot.md)

Slime-module

- [Lazyload Usage](https://github.com/slime-io/lazyload/blob/master/README.md)
- [PluginManager Usage](./doc/en/plugin_manager.md)
- [SmartLimiter Usage](./doc/en/smart_limiter.md)





## Community

- Slack: [https://slimeslime-io.slack.com/invite](https://join.slack.com/t/slimeslime-io/shared_invite/zt-u3nyjxww-vpwuY9856i8iVlZsCPtKpg)

- email: slimedotio@gmail.com

- QQ Group: 971298863 

  <img src="media/slime-qq.png" alt="slime qq group" style="zoom: 50%;" /> 





## License

[Apache-2.0](https://choosealicense.com/licenses/apache-2.0/)

