#!/usr/bin/env bash
set -x
mkdir -p ./helm-charts/slimeboot/templates/modules
MODS=${MODS:-"lazyload limiter plugin"}
for m in $MODS; do
  rm -rf "./helm-charts/slimeboot/templates/modules/$m"
  cp -r "../staging/src/slime.io/slime/modules/$m/charts/" "./helm-charts/slimeboot/templates/modules/$m"
  rm -rf "./helm-charts/slimeboot/templates/modules/$m/crds"
done
find ./helm-charts/slimeboot/templates/modules -type f | grep -v ".yaml" | xargs --no-run-if-empty  rm -f 
for e in Chart.yaml values.yaml; do
  find ./helm-charts/slimeboot/templates/modules -type f -name "$e" -delete
done

export MOD=boot
export ALL_ACTIONS="image image-push"

if [[ "$1" == "publish" || "$1" == "copy"  ]]; then
  ../bin/multiarch.sh "$@"
else
  ../bin/publish.sh "$@"
fi
