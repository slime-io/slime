#!/bin/bash
kubectl create ns mesh-operator
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/75ed452f5fdba82dfde0d3be364bee30b6056072/install/init/crds.yaml
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/75ed452f5fdba82dfde0d3be364bee30b6056072/install/init/slime-boot-install.yaml
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/75ed452f5fdba82dfde0d3be364bee30b6056072/install/samples/smartlimiter/easy_install_limiter.yaml