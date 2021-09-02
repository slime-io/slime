#!/bin/bash
for i in $(kubectl get ns);do kubectl delete smartlimiter -n $i --all;done
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/v0.2.1/install/samples/smartlimiter/slimeboot_smartlimiter.yaml
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/v0.2.1/install/init/deployment_slime-boot.yaml
kubectl delete -f https://raw.githubusercontent.com/slime-io/slime/v0.2.1/install/init/crds.yaml
kubectl delete ns mesh-operator