#!/usr/bin/env bash

export MOD=bundle-example-all

if [[ "$1" == "publish" ]]; then
  ../../bin/multiarch.sh "$@"
else
  ../../bin/publish.sh "$@"
fi
