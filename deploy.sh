#!/usr/bin/env bash
export HUB=docker.io/bcxq/slime
branch=$(git symbolic-ref --short -q HEAD)
commit=$(git rev-parse --short HEAD)
tag=$(git show-ref --tags| grep $commit | awk -F"[/]" '{print $3}')
if [ -z $tag ]
then
  operator-sdk build $HUB:$branch-$commit
  docker push $HUB:$branch-$commit
else
  operator-sdk build $HUB:$tag
  docker push $HUB:$tag
fi