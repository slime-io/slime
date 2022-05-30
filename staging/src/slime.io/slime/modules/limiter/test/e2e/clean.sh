#!/bin/bash

sed -e 's/{{istioRevKey}}/istioRevKey/g' -e 's/{{istioRevValue}}/istioRevValue/g' ./testdata/install/samples/limit/productpage_smartlimiter.yaml > tmp_productpage_smartlimiter.yaml
kubectl delete -f tmp_productpage_smartlimiter.yaml

sed -e 's/{{limitTag}}/limitTag/g' ./testdata/install/samples/limit/slimeboot_limit.yaml > tmp_slimeboot_limit.yaml
kubectl delete -f tmp_slimeboot_limit.yaml

kubectl delete -f ./testdata/install/config/bookinfo.yaml

sed -e 's/{{slimebootTag}}/slimebootTag/g' ./testdata/install/init/deployment_slime-boot.yaml > tmp_deployment_slime-boot.yaml
kubectl delete -f tmp_deployment_slime-boot.yaml

kubectl delete -f ./testdata/install/init/crds.yaml

kubectl delete ns temp

kubectl delete ns mesh-operator

rm -f tmp_*
