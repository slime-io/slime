#!/bin/bash
# use sed cmd change {{xx}} to xx in yaml files
# the value of xx has no effect on cleaning resource

sed -e 's/{{istioRevKey}}/istioRevKey/g' -e 's/{{istioRevValue}}/istioRevValue/g' ./testdata/install/samples/lazyload/servicefence_productpage.yaml > tmp_servicefence_productpage.yaml
kubectl delete -f tmp_servicefence_productpage.yaml

sed -e 's/{{lazyloadTag}}/lazyloadTag/g' -e 's/{{istioRevValue}}/istioRevValue/g' -e 's/{{strictRev}}/strictRev/g' -e 's/{{globalSidecarTag}}/globalSidecarTag/g' -e 's/{{globalSidecarPilotTag}}/globalSidecarPilotTag/g' ./testdata/install/samples/lazyload/slimeboot_lazyload.yaml > tmp_slimeboot_lazyload.yaml
kubectl delete -f tmp_slimeboot_lazyload.yaml

kubectl delete -f ./testdata/install/config/bookinfo.yaml

sed -e 's/{{slimebootTag}}/slimebootTag/g' ./testdata/install/init/deployment_slime-boot.yaml > tmp_deployment_slime-boot.yaml
kubectl delete -f tmp_deployment_slime-boot.yaml

kubectl delete -f ./testdata/install/init/crds.yaml

kubectl delete ns example-apps

kubectl delete ns mesh-operator

rm -f tmp_*
