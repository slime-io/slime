#!/usr/bin/env bash

export MOD=lazyload

if [[ "$1" == "publish" || "$1" == "copy"  ]]; then
  ../../../../../../bin/multiarch.sh "$@"
else
  ../../../../../../bin/publish.sh "$@"
fi
