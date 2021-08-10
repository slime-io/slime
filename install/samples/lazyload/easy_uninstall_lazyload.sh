#!/bin/bash
for i in $(kubectl get ns);do kubectl delete servicefence -n $i --all;done
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/3101372ec2197dd99f8d7a49d04ccf9430ebed1d/install/samples/lazyload/easy_install_lazyload.yaml
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/3101372ec2197dd99f8d7a49d04ccf9430ebed1d/install/init/slime-boot-install.yaml
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/3101372ec2197dd99f8d7a49d04ccf9430ebed1d/install/init/crds.yaml
kubectl delete ns mesh-operator