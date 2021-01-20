#!/bin/bash
if [ $(kubectl get ns mesh-operator | grep mesh-operator) = "" ];then
kubectl create ns mesh-operator
fi
kubectl apply -f https://raw.githubusercontent.com/ydh926/slime/master/install/crds.yaml
kubectl apply -f https://raw.githubusercontent.com/ydh926/slime/master/install/slime-boot-install.yaml
kubectl apply -f https://raw.githubusercontent.com/ydh926/slime/master/install/config/lazyload_install.yaml --validate=false
