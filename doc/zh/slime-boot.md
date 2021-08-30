- [安装slime-boot](#安装slime-boot)
- [安装Prometheus](#安装prometheus)
- [验证](#验证)

### 

## 安装slime-boot

在使用slime之前，需要安装slime-boot，通过slime-boot，可以方便的安装和卸载slime模块。 执行如下命令：

```shell
$ kubectl create ns mesh-operator
$ kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/init/crds.yaml
$ kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/init/deployment_slime-boot.yaml
```



## 安装Prometheus

slime的懒加载和自适应等模块配合监控指标使用方便，建议部署Prometheus。这里提供一份istio官网的简化部署文件拷贝。

```shell
$ kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/config/prometheus.yaml
```



## 验证

安装完毕后，检查mesh-operator中创建的slime-boot与istio-system中的prometheus。

```sh
$ kubectl get po -n mesh-operator
NAME                         READY   STATUS    RESTARTS   AGE
slime-boot-fd9d7ff6d-4qb5f   1/1     Running   0          3h25m
$ kubectl get po -n istio-system
NAME                                    READY   STATUS    RESTARTS   AGE
istio-egressgateway-78cb6c4799-6w2cn    1/1     Running   5          14d
istio-ingressgateway-59644976b5-kmw9s   1/1     Running   5          14d
istiod-664799f4bc-wvdhv                 1/1     Running   5          14d
prometheus-69f7f4d689-hrtg5             2/2     Running   2          4d4h
```

