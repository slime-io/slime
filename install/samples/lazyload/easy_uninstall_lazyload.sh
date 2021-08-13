#!/bin/bash
for i in $(kubectl get ns);do kubectl delete servicefence -n $i --all;done
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/v0.1.2/install/samples/lazyload/easy_install_lazyload.yaml
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/v0.1.2/install/init/slime-boot-install.yaml
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/v0.1.2/install/init/crds.yaml
kubectl delete ns mesh-operator