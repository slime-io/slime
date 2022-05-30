#!/usr/bin/env bash

MODS=${MODS:-"lazyload limiter plugin"}
for m in $MODS; do
  rm -rf "./helm-charts/slimeboot/templates/modules/$m"
  cp -r "../staging/src/slime.io/slime/modules/$m/charts/" "./helm-charts/slimeboot/templates/modules/$m"
done

export MOD=boot
export ALL_ACTIONS="image image-push"
../bin/publish.sh "$@"
