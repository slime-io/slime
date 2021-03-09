#!/bin/bash
kubectl create ns mesh-operator
kubectl apply -f ../../init/crds.yaml
kubectl apply -f ../../init/slime-boot-install.yaml
kubectl apply -f ../../config/limiter_install_with_metric.yaml --validate=false