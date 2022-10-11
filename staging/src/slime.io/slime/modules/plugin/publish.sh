#!/usr/bin/env bash

source *.env.sh 2>/dev/null
export MOD=plugin
../../../../../../bin/publish.sh "$@"
