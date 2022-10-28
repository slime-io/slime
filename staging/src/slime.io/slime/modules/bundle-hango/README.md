## 背景
`hango`目前使用了`slime`的`plugin`和`limiter`模块，本文档提供`hango`场景下`slime`的部署方式

## 安装

1. 安装`slime`组件前，需要安装相关`CRD

需要用户手动 apply [v1版本](install/crds-v1.yaml) 或者 [v1beta1版本](install/crds-v1beta1.yaml) 版本的`CRD`

**注意**：

k8s version >= v1.22，只支持`apiextensions.k8s.io/v1`版本的`CRD`，不再支持`apiextensions.k8s.io/v1beta1`版本`CRD`，详见[k8s官方文档](https://kubernetes.io/docs/reference/using-api/deprecation-guide/#customresourcedefinition-v122)

1.16 <= k8s version < v1.22 两个版本选其一

2. 安装`slime`组件，使用[deployment](install/slime-hango.yaml)
