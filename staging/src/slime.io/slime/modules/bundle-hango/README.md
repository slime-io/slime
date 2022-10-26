## 背景
`hango`目前使用了`slime`的`plugin`和`limiter`模块，本文档提供`hango`场景下`slime`的部署方式

## 安装

1. 安装`slime`组件前，需要安装相关`CRD`

对于v1.22及以上版本，使用[v1版本](install/crds-v1.yaml)， 对于v1.22以下使用[v1beta1版本](install/crds-v1.yaml)

2. 安装`slime`组件，使用[deployment](install/slime-hango.yaml)
