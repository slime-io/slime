#!/usr/bin/env bash

HUB=${HUB:-"docker.io/slimeio"}

function fatal() {
  echo "$1" >&2
  exit 1
}

if test -z "$MOD"; then
  fatal "empty MOD"
fi

version=$(cat VERSION)  # get version from file
commit=$(git rev-parse --short HEAD)
if [[ -z "${version}" ]]; then
  tag=$(git show-ref --tags| grep "$commit" | awk -F"[/]" '{print $3}')
  if [[ -z "${tag}" ]]; then
    branch=$(git symbolic-ref --short -q HEAD)
    if [[ -n "${branch}" ]]; then
      version=$branch
    fi
  else
    version=$tag  # use HEAD tag as version
  fi
fi
if [ -z "$version" ]; then
  image_tag="$commit"
else
  image_tag="$version-$commit"
fi

image_url="$HUB/slime-$MOD:$image_tag"

ALL_ACTIONS="build image push"

actions=
if [[ "$#" -eq 0 ]]; then
  echo "no action. supported actions: \"$ALL_ACTIONS\" or pass ALL to indicate all actions" >&2
  exit
elif [[ "$1" == "ALL" ]]; then
  actions="$ALL_ACTIONS"
else
  actions="$*"
fi

export GOOS=linux
export GOARCH=amd64

function print_info() {
  for info in "$@"; do
    case "$info" in
    image)
      echo -e "image:\n  image_url: ${image_url}"
      ;;
    *)
      echo "unknown info: ${info}" >&2
    esac
  done
}

set -x
for action in $actions; do
  case "$action" in
  build)
    go build -o manager.exe
    ;;
  image)
    docker build -t "${image_url}" .
    ;;
  print)  # should be the only action
    # rest param will be consider as info to print, like: *.sh print image
    set +x
    shift
    print_info "$@"
    break
    ;;
  push)
    docker push "${image_url}"
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
set +x
