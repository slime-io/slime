#!/usr/bin/env bash

export MOD=bundle-hango
if [[ "$1" == "publish" ]]; then
  ../../../../../../bin/multiarch.sh "$@"
else
  ../../../../../../bin/publish.sh "$@"
fi
