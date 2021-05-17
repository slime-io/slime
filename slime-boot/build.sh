#!/usr/bin/env bash
branch=$(git symbolic-ref --short -q HEAD)
commit=$(git rev-parse --short HEAD)
tag=$(git show-ref --tags| grep $commit | awk -F"[/]" '{print $3}')
if [ -z $tag ]
then
  operator-sdk build $HUB/slime-boot:$branch-$commit
else
  operator-sdk build $HUB/slime-boot:$tag
  docker push $HUB/slime-lazyload:$tag
fi
