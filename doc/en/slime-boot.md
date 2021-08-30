- [How to Use Slime](#how-to-use-slime)
  - [Install Slime-boot](#install-slime-boot)
  - [Install Prometheus](#install-prometheus)
  - [Verify](#verify)

## How to Use Slime

### Install Slime-boot

You can easily install and uninstall the slime sub-module with slime-boot. Using the following commands to install slime-boot:

```sh
$ kubectl create ns mesh-operator
$ kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/init/crds.yaml
$ kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/init/deployment_slime-boot.yaml
```



### Install Prometheus

The lazy load and smart limiter module needs metric data, so we suggest you installing prometheus in your system. Here is a simple prometheus installation file copied from istio.io.

```sh
$ kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.2.0-alpha/install/config/prometheus.yaml
```



### Verify

After installation of slime-boot, check whether slime-boot pod in mesh-operator and prometheus pod in istio-system are running. 

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

