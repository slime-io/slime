#!/usr/bin/env bash
export HUB=docker.io/bcxq/slime-plugin
branch=$(git symbolic-ref --short -q HEAD)
commit=$(git rev-parse --short HEAD)
tag=$(git show-ref --tags| grep $commit | awk -F"[/]" '{print $3}')
go build
mv plugin manager
if [ -z $tag ]
then
  docker build -t $HUB:$branch-$commit .
  docker push $HUB:$branch-$commit
else
  docker build -t $HUB:$tag .
  docker push $HUB:$tag
fi