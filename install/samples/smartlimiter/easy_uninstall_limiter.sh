#!/bin/bash
for i in $(kubectl get ns);do kubectl delete smartlimiter -n $i --all;done
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/75ed452f5fdba82dfde0d3be364bee30b6056072/install/samples/smartlimiter/easy_install_limiter.yaml
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/75ed452f5fdba82dfde0d3be364bee30b6056072/install/init/slime-boot-install.yaml
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/75ed452f5fdba82dfde0d3be364bee30b6056072/install/init/crds.yaml
kubectl delete ns mesh-operator