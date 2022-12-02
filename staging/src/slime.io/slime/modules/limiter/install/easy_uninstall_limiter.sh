#!/bin/bash
if [[ "$#" -eq 0 ]]; then
  echo "No specified tag or commit. Use the latest tag."
  slime_tag_or_commit=$(curl -s https://api.github.com/repos/slime-io/slime/tags | grep 'name' | cut -d\" -f4 | head -1)
  if [[ -z $slime_tag_or_commit ]]; then
    echo "Failed to get the latest slime tag. Exited."
    exit 1
  fi
  echo "The Latest slime tag: $slime_tag_or_commit."
fi

crds_url="https://raw.githubusercontent.com/slime-io/slime/$slime_tag_or_commit/install/init/crds.yaml"
deployment_slimeboot_url="https://raw.githubusercontent.com/slime-io/limiter/$slime_tag_or_commit/install/init/deployment_slime-boot.yaml"
slimeboot_smartlimiter_url="https://raw.githubusercontent.com/slime-io/slime/$slime_tag_or_commit/staging/src/slime.io/slime/modules/limiter/install/limiter.yaml"

for i in $(kubectl get ns --no-headers |awk '{print $1}');do kubectl delete smartlimiter -n $i --all;done
kubectl delete -f "${slimeboot_smartlimiter_url}"
kubectl delete -f "${deployment_slimeboot_url}"
kubectl delete -f "${crds_url}"
kubectl delete ns mesh-operator
