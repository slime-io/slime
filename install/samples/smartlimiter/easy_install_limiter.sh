#!/bin/bash
kubectl create ns mesh-operator
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/3101372ec2197dd99f8d7a49d04ccf9430ebed1d/install/init/crds.yaml
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/3101372ec2197dd99f8d7a49d04ccf9430ebed1d/install/init/slime-boot-install.yaml
kubectl apply -f https://raw.githubusercontent.com/slime-io/slime/3101372ec2197dd99f8d7a49d04ccf9430ebed1d/install/samples/smartlimiter/easy_install_limiter.yaml