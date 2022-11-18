#!/usr/bin/env bash

source *.env.sh 2>/dev/null
export MOD=plugin
if [[ "$1" == "publish" || "$1" == "copy"  ]]; then
  ../../../../../../bin/multiarch.sh "$@"
else
  ../../../../../../bin/publish.sh "$@"
fi

