#!/bin/bash
kubectl create ns mesh-operator
kubectl apply -f https://raw.githubusercontent.com/ydh926/slime/master/install/crds.yaml
kubectl apply -f https://raw.githubusercontent.com/ydh926/slime/master/install/slime-boot-install.yaml
kubectl apply -f https://raw.githubusercontent.com/ydh926/slime/master/install/config/limiter_install.yaml --validate=false