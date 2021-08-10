#!/bin/bash
for i in $(kubectl get ns);do kubectl delete smartlimiter -n $i --all;done
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/3101372ec2197dd99f8d7a49d04ccf9430ebed1d/install/samples/smartlimiter/easy_install_limiter.yaml
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/3101372ec2197dd99f8d7a49d04ccf9430ebed1d/install/init/slime-boot-install.yaml
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/3101372ec2197dd99f8d7a49d04ccf9430ebed1d/install/init/crds.yaml
kubectl delete ns mesh-operator