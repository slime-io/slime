## 版本支持

Slime项目依靠Istio的Envoyfilter、Sidecar资源，无侵入式扩展Isito功能。

Istio社区采用的是维护最近的三个大版本策略，Istio相关数据如下

| Version | Currently Supported | Release Date | End of Life | Supported Kubernetes Versions | Tested, but not supported |
| ------- | ------------------- | ------------ | ----------- | ----------------------------- | -------------------------- |
| 1.19    | Yes                 | Sept 5, 2023 | ~March 2024 (Expected) | 1.25, 1.26, 1.27, 1.28 | 1.21, 1.22, 1.23, 1.24 |
| 1.18    | Yes                 | Jun 3, 2023  | ~Dec 2023 (Expected)   | 1.24, 1.25, 1.26, 1.27 | 1.20, 1.21, 1.22, 1.23 |
| 1.17    | Yes                 | Feb 14, 2023 | Oct 27, 2023           | 1.23, 1.24, 1.25, 1.26 | 1.16, 1.17, 1.18, 1.19, 1.20, 1.21, 1.22 |
| 1.16    | No                  | Nov 15, 2022 | Jul 25, 2023           | 1.22, 1.23, 1.24, 1.25 | 1.16, 1.17, 1.18, 1.19, 1.20, 1.21 |
| 1.15    | No                  | Aug 31, 2022 | Apr 4, 2023            | 1.22, 1.23, 1.24, 1.25 | 1.16, 1.17, 1.18, 1.19, 1.20, 1.21 |
| 1.14    | No                  | May 24, 2022 | Dec 27, 2022           | 1.21, 1.22, 1.23, 1.24 | 1.16, 1.17, 1.18, 1.19, 1.20 |
| 1.13    | No                  | Feb 11, 2022 | Oct 12, 2022           | 1.20, 1.21, 1.22, 1.23 | 1.16, 1.17, 1.18, 1.19 |


Slime社区对Istio的支持版本如下:

**近期对Istio的主要版本（>=1.13.0）进行了测试，Slime各项功能均能可靠运行**

**其中个别低版本的兼容问题，可参考 [#145](https://github.com/slime-io/slime/issues/145)**

Slime社区对Kubernetes的支持版本如下:

跟随Istio社区对Kubernetes的支持版本，Slime社区对Kubernetes版本要求如下：

**1.16.0 <= k8s版本要求 <= 1.25.0**

其中需要注意的是，在k8s v1.22以及之后的版本中，只支持apiextensions.k8s.io/v1版本的CRD，不再支持apiextensions.k8s.io/v1beta1版本CRD， 因此在k8s v1.22以及之后的版本中，需要将slime的crd版本升级到v1版本，具体方法参考 [slime安装准备](https://github.com/slime-io/slime/blob/master/doc/zh/slime-boot.md#%E5%87%86%E5%A4%87)
