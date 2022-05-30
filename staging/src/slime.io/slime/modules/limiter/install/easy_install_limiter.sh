#!/bin/bash
if [[ "$#" -eq 0 ]]; then
  echo "No specified tag or commit. Use the latest tag."
  slime_tag_or_commit=$(curl -s https://api.github.com/repos/slime-io/slime/tags | grep 'name' | cut -d\" -f4 | head -1)
  limiter_tag_or_commit=$(curl -s https://api.github.com/repos/slime-io/limiter/tags | grep 'name' | cut -d\" -f4 | head -1)
  if [[ -z $slime_tag_or_commit ]]; then
    echo "Failed to get the latest slime tag. Exited."
    exit 1
  fi
  if [[ -z $limiter_tag_or_commit ]]; then
    echo "Failed to get the latest limiter tag. Exited."
    exit 1
  fi
  echo "The Latest tag: $slime_tag_or_commit."
  echo "The Latest tag: $limiter_tag_or_commit."
else
  slime_tag_or_commit=$1
  limiter_tag_or_commit=$2
  echo "Use specified slime tag or commit: $slime_tag_or_commit"
  echo "Use specified limiter tag or commit: $limiter_tag_or_commit"
fi

crds_url="https://raw.githubusercontent.com/slime-io/slime/$slime_tag_or_commit/install/init/crds.yaml"
deployment_slimeboot_url="https://raw.githubusercontent.com/slime-io/slime/$slime_tag_or_commit/install/init/deployment_slime-boot.yaml"
slimeboot_smartlimiter_url="https://raw.githubusercontent.com/slime-io/limiter/master/install/limiter.yaml"

kubectl create ns mesh-operator
kubectl apply -f "${crds_url}"
kubectl apply -f "${deployment_slimeboot_url}"
kubectl apply -f "${slimeboot_smartlimiter_url}"
