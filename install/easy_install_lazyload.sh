#!/bin/bash
kubectl create ns mesh-operator
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/master/install/crds.yaml
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/master/install/slime-boot-install.yaml
kubectl apply -f config/lazyload_install_with_metric.yaml --validate=false
