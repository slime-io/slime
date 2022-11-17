#!/usr/bin/env bash

export MOD=global-sidecar

if [[ "$1" == "publish" || "$1" == "copy"  ]]; then
  ../../../../../../../../bin/multiarch.sh "$@"
else
  ../../../../../../../../bin/publish.sh "$@"
fi