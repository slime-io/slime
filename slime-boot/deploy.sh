#!/usr/bin/env bash
export HUB=docker.io/hazard1905/slime-boot
branch=$(git symbolic-ref --short -q HEAD)
commit=$(git rev-parse --short HEAD)
tag=$(git show-ref --tags| grep $commit | awk -F"[/]" '{print $3}')
if [ -z $tag ]
then
  docker_tag="$HUB:$branch-$commit"
else
  docker_tag="$HUB:$tag"
fi
docker build -f build/Dockerfile -t "$docker_tag" .
docker push "$docker_tag"

