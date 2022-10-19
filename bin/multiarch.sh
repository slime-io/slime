#!/usr/bin/env bash

# usage: create multiarch image
#   PREFIX="docker.io/slimeio/slime-limiter:" ./multiarch.sh --push v1 \
#   v1_linux_amd64 v1_linux_arm64
##   or
#   ./multiarch.sh --push docker.io/slimeio/slime-limiter:v1 \
#   slimeio/slime-limiter:v1_linux_arm64 \
#   slimeio/slime-limiter:v1_linux_amd64

# usage: deploy multiarch image for submodules
## will to build,image,push,manifest-create,manifest-push for specified archs and target
#  ./multiarch.sh publish arm64 amd64

set -o errexit

if [[ "$1" == "publish" ]]; then
  shift
  pwd
  image_no_arch=$(./publish.sh print-image-noarch)
  images=()
  for arch in "$@"; do
    echo "deploy of arch $arch"
    TARGET_GOARCH=$arch ./publish.sh ALL
    images+=("$(TARGET_GOARCH=$arch ./publish.sh print-image)")
  done

   $0 --push "$image_no_arch" "${images[@]}"
  exit
fi

# example:
#   ./multiarch.sh copy docker.io/slimeio/slime-limiter:v1 \
#   registry.cn-hangzhou.aliyuncs.com/slimeio/slime-limiter:v1
if [[ "$1" == "copy" ]]; then
  shift
  from="$1"
  to="$2"

  which jq || {
    echo "no jq, plz install it" >&2
    exit 1
  }

  from_parts=(${from//:/ })
  from_=${from_parts[0]}  # image without tag

  docker manifest inspect "$from" | jq -r '.manifests[] | .digest + " " + .platform.os + "_" + .platform.architecture' | {
    arr=()
    while read -r -a row_arr; do
      sha=${row_arr[0]}
      arch=${row_arr[1]}
      to_arch="${to}_${arch}"
      docker tag "${from_}@${sha}" "$to_arch"  # sha is more precise as there's no tag in manifest info.
      docker push "$to_arch"
      echo "add $to_arch"
      arr+=("$to_arch")
    done

   ../slime/bin/multiarch.sh --push "$to" "${arr[@]}"
  }

  exit
fi

push=
if [[ "$1" == "--push" ]]; then
  push=1
fi
shift

target="$1"
if [[ -n "$PREFIX" ]]; then
  target="$PREFIX$target"
fi
shift

sources=()

for src in "$@"; do
  if [[ -n "$PREFIX" ]]; then
    src="$PREFIX$src"
  fi
  sources+=("$src")
done

echo "create multiarch image $target from ${sources[*]}"
docker manifest create --amend "$target" "${sources[@]}"

if [[ -n "$push" ]]; then
  echo "push multiarch image $target"
  docker manifest push "$target"
fi
