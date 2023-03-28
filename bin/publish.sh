#!/usr/bin/env bash

# usage
# `sh ./publish.sh ALL` in each submodules

source "$(dirname $0)"/*.env.sh 2>/dev/null

HUB=${HUB:-"docker.io/slimeio registry.cn-hangzhou.aliyuncs.com/slimeio"}
PUSH_HUBS="$HUB"
first_hub=$(echo $HUB | awk -F " " '{print $1}')
TARGET_GOARCH=${TARGET_GOARCH:-${GOARCH:-amd64}}
TARGET_GOOS=${TARGET_GOOS:-${GOOS:-linux}}
CGO_ENABLED=${CGO_ENABLED:-0}
BASE_IMAGE=${BASE_IMAGE-"ubuntu:focal"}

export GO111MODULE=on

function fatal() {
  echo "$1" >&2
  exit 1
}

function calc_unstaged_hash() {
  local tmp_f
  tmp_f=$(mktemp)
  cp $(git rev-parse --show-toplevel)/.git/index "$tmp_f"
  GIT_INDEX_FILE="$tmp_f" git add -u
  GIT_INDEX_FILE="$tmp_f" git write-tree
}

if test -z "$MOD"; then
  fatal "empty MOD"
fi

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

MOD="slime-$MOD"

dirty=
if [[ -z "${IGNORE_DIRTY}" && -n "$(git status -s --porcelain)" ]]; then
  unstaged_hash=$(calc_unstaged_hash)
  dirty="-dirty_${unstaged_hash::7}"
fi

commit=$(git rev-parse --short HEAD)

if [[ -n "$GIT_TAG" ]]; then
  echo "checkout to specified git tag: $GIT_TAG" >&2
  git checkout "$GIT_TAG"
fi

if [[ -z "$TAG" ]]; then
  branch=$(git symbolic-ref --short -q HEAD)
  tag=${GIT_TAG:-$(git show-ref --tags | grep "$commit" | awk -F"[/]" '{print $3}' | tail -1)}
  if [ -z "$tag" ]; then
    if [ -z "$branch" ]; then
      export TAG="$commit" # detach case
    else
      export TAG="$branch-$commit"
    fi
  else
    export TAG=$tag
  fi

  TAG_NO_ARCH=$TAG
  TAG=${TAG}_${TARGET_GOOS}_${TARGET_GOARCH}

  if test -z "${IGNORE_DIRTY}" && test -n "$(git status -s --porcelain)"; then
    TAG=$TAG${dirty}
    TAG_NO_ARCH=$TAG_NO_ARCH${dirty}
  fi
else
  TAG_NO_ARCH=$TAG
fi

image="${first_hub}/$MOD:${TAG}"
image_no_arch="${first_hub}/$MOD:${TAG_NO_ARCH}"
image_url=${image}
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
    CGO_ENABLED="${CGO_ENABLED}" GOOS="${TARGET_GOOS}" GOARCH=${TARGET_GOARCH} go build -o manager.exe
    ;;
  image)
    echo "build docker image: ${image}" >&2
    docker buildx build --platform ${TARGET_GOOS}/${TARGET_GOARCH} --load --build-arg BASE_IMAGE=$BASE_IMAGE -t ${image} .
    ;;
  image-push)
    for push_hub in ${PUSH_HUBS}; do
      push_url="${push_hub}/$MOD:${TAG}"
      if [[ "${push_url}" != "${image_url}" ]]; then
        docker tag "${image_url}" "${push_url}"
      fi
      echo "push image $push_url"
      docker push "${push_url}"
    done
    ;;
  print-image) # should be the only action
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
