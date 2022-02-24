#!/usr/bin/env bash

# usage
# `sh ./publish.sh ALL` in each submodules

HUB=${HUB:-"docker.io/liuliluo"}
TARGET_GOARCH=${TARGET_GOARCH:-${GOARCH:-amd64}}
TARGET_GOOS=${TARGET_GOOS:-${GOOS:-linux}}
CGO_ENABLED=${CGO_ENABLED:-0}
export GO111MODULE=on

if [[ -z "$TARGET_GOOS" ]]; then
  if uname | grep -q Darwin; then
    TARGET_GOOS=darwin
  else
    TARGET_GOOS=linux
  fi
fi

if test -z "$MOD"; then
  fatal "empty MOD"
fi

if [[ -z "$TAG" ]]; then
  branch=$(git symbolic-ref --short -q HEAD)
  tag=$(git tag --points-at HEAD)
  if [ -z "$tag" ]
  then
    commit=$(git rev-parse --short HEAD)
    if [ -z "$branch" ]; then
      export TAG="$commit"  # detach case
    else
      export TAG="$branch-$commit"
    fi
  else
     export TAG=$tag
  fi

  TAG_NO_ARCH=$TAG
  TAG=${TAG}_${TARGET_GOOS}_${TARGET_GOARCH}

  if test -z "${IGNORE_DIRTY}" && test -n "$(git status -s --porcelain)"; then
    TAG=$TAG-dirty
    TAG_NO_ARCH=$TAG_NO_ARCH-dirty
  fi
else
  TAG_NO_ARCH=$TAG
fi

image="${HUB}/$MOD:${TAG}"
image_no_arch="${HUB}/$MOD:${TAG_NO_ARCH}"

ALL_ACTIONS=${ALL_ACTIONS:-"build image image-push"}

actions=
if [[ "$#" -eq 0 ]]; then
  echo "no action. supported actions: \"$ALL_ACTIONS\" or pass ALL to indicate all actions" >&2
  exit
elif [[ "$1" == "ALL" ]]; then
  actions="$ALL_ACTIONS"
else
  actions="$*"
fi

for action in $actions; do
  case "$action" in
  build)
    echo "go build submodules ${MOD}"
    CGO_ENABLED="${CGO_ENABLED}" GOOS="${TARGET_GOOS}" GOARCH=${TARGET_GOARCH}  go build -o manager.exe
    ;;
  image)
    echo "build docker image: ${image}" >&2
    if [[ "$TARGET_GOOS" == "linux" && "$TARGET_GOARCH" == "amd64" && ("$(uname -p)" == "x86_64" || "$(uname -p)" == "i386") ]]; then
      docker build --platform ${TARGET_GOOS}/${TARGET_GOARCH} -t ${image} .
    else
      docker buildx build --platform ${TARGET_GOOS}/${TARGET_GOARCH} --load -t ${image} .
    fi
    ;;
  image-push)
    echo "push image $image"
    docker push $image
    ;;
  print-image)  # should be the only action
    echo "$image"
    ;;
  print-image-noarch)
    echo "$image_no_arch"
    ;;
  *)
    echo "skip unknown action $action"
    ;;
  esac

  step_exit=$?
  if [[ "${step_exit}" -ne 0 ]]; then
    exit ${step_exit}
  fi
done
