#!/bin/bash
kubectl create ns mesh-operator
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.1.2/install/init/crds.yaml
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.1.2/install/init/slime-boot-install.yaml
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/v0.1.2/install/samples/lazyload/easy_install_lazyload.yaml