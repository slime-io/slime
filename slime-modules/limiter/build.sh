#!/usr/bin/env bash
branch=$(git symbolic-ref --short -q HEAD)
commit=$(git rev-parse --short HEAD)
tag=$(git show-ref --tags| grep $commit | awk -F"[/]" '{print $3}')
go build
mv limiter manager
if [ -z $tag ]
then
  docker build -t $HUB/slime-limiter:$branch-$commit .
else
  docker build -t $HUB/slime-limiter:$tag .
  docker push $HUB/slime-limiter:$tag
fi