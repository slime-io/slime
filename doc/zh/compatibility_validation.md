## 背景

istio与slime都在向前演进，本文档将记录slime与最新istio版本的兼容性


## istio 安装

1. 下载最新版本的istioctl [address](https://github.com/istio/istio/releases)
   ```
   istioctl install --revision test-16 --set profile=demo
   ```

2. 安装slimeboot, 可参考 [install slimeboot](https://github.com/istio/istio/releases/download/1.16.0/istio-1.16.0-linux-amd64.tar.gz)

3. 安装v0.5.0版本bundle镜像（包含lazyload/limiter/plugin功能）[bundle](https://github.com/slime-io/slime/blob/master/doc/zh/slime-boot.md#bundle%E6%A8%A1%E5%BC%8F%E5%AE%89%E8%A3%85%E6%A0%B7%E4%BE%8B)

4. 部署bookinfo


## lazyload验证

1. 给productpage 的ns打上istio.io/rev: test-16, 重启ns下所有pods

2. 给productpage的svc打上slime.io/serviceFenced,观察是否生成servicefence和sidecar

3. 在ratings服务中执行 `curl productpage:9080/productpage -I`

4. 观察productpage的accesslog, 流量走到了global-sidecar

5. 在ratings服务中再次执行 `curl productpage:9080/productpage -I`

6. 看productpage的accesslog, 流量走到了各自路由


## limiter验证

1. 给productpage设置 60s/2次 限流规则 [smartlimiter](../../staging/src/slime.io/slime/modules/limiter/install/limiter.instance.yaml)

2. 在 rating 中多次执行 `curl productpage:9080/productpage -I`



|            | slime-v0.5.0 |
| ---------- | ------------ |
| istio-1.12 | 功能正常     |
| istio-1.13 | 功能正常     |
| istio-1.14 | 功能正常     |
| istio-1.15 | 功能正常     |
| istio-1.16 | 功能正常     |

