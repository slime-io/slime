#!/usr/bin/env bash

MODS=${MODS:-"lazyload limiter plugin"}
for m in $MODS; do
  rm -rf "./helm-charts/slimeboot/templates/modules/$m"
  cp -r "../../$m/charts/" "./helm-charts/slimeboot/templates/modules/$m"
done

export MOD=boot
export ALL_ACTIONS="image pushAll"
../bin/publish.sh "$@"
