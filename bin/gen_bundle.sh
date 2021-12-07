#!/bin/bash

# usage example: bash gen_bundle.sh my_bundle lazyload limiter
# get $MPATH/src/my_bundle

BPATH=${BPATH:-$GOPATH}
echo "Bundle path is $BPATH"

# get bundle and module names
if [[ "$#" -lt 2 ]]; then
  echo "Not enough params: one bundle name and at least 1 module names. Please provide them and try again."
  exit 1
else
  bundle_name=$1
  echo "bundle name: $bundle_name"
  shift
  modules=("$@")
  echo "module names: ${modules[@]}"
fi

# check module directory
for module in ${modules[@]}; do
  if [ ! -d "$BPATH/src/$module" ]; then
    echo "Error: $BPATH/src/$module does not exist"
    exit 1
  fi
done

# check bundle directory
if [ -d "$BPATH/src/$bundle_name" ]; then
  read -p "$BPATH/src/$bundle_name already exists. Delete? (yes/no)" delete
  if [[ "$delete" != "yes" ]]; then
    echo "Exit without actions."
    exit 1
  else
    echo "Delete old $BPATH/src/$bundle_name directory."
    rm -rf "$BPATH/src/$bundle_name"
  fi
fi

# generate bundle files
cp -r "$BPATH/src/slime/modules/bundle_example" "$BPATH/src/$bundle_name"
cd "$BPATH/src/$bundle_name"

function replace_go_mod() {
  sed -i "s/bundle_example/$bundle_name/g" go.mod
  sed -i "s/slime.io\/slime\/framework => ..\/..\/framework/slime.io\/slime\/framework => ..\/slime\/framework/g" go.mod

  sed -i "20,22d" go.mod
  sed -i "6,8d" go.mod

  for module in ${modules[@]}; do
    sed -i "16a\    \slime.io/slime/modules/$module => ../$module" go.mod
    #sed -i "17s/^/\t/" go.mod
  done
  for module in ${modules[@]}; do
    sed -i "5a\    \slime.io/slime/modules/$module v0.0.0" go.mod
    #sed -i "6s/^/\t/" go.mod
  done

  echo "replace go.mod"
}

# replace go.mod
replace_go_mod

# replace main.go
function replace_main_go() {
  sed -i "28,30d" main.go
  sed -i "21,23d" main.go
  for module in ${modules[@]}; do
    sed -i "24a &${module}mod.Module{}," main.go
    sed -i "25s/^/\t\t/" main.go
  done
  for module in ${modules[@]}; do
    sed -i "20a ${module}mod \"slime.io/slime/modules/$module/module\"" main.go
    sed -i "21s/^/\t/" main.go
  done
  echo "replace main.go"
}
replace_main_go

# replace publish.sh
function replace_publish_go() {
  sed -i "s/bundle-example-all/$bundle_name/g" publish.sh
  sed -i "s/..\/..\/bin\/publish.sh/..\/slime\/bin\/publish.sh/" publish.sh
  echo "replace publish.sh"
}
replace_publish_go

# replace install
function replace_bundle_example_yaml() {
  sed -i "s/bundle-example-all/$bundle_name/g" install/bundle_example.yaml
  sed -i '17,$d' install/bundle_example.yaml
  for module in ${modules[@]}; do
    sed -i "\$a\          \- name: $module" install/bundle_example.yaml
    #sed -i "\$s/^/          /" install/bundle_example.yaml
  done
  for module in ${modules[@]}; do
    sed -i "\$a\    \- name: $module\n\
      enable: true\n\
      mode: BundleItem" install/bundle_example.yaml
  done
  echo "replace install/bundle_example.yaml"
}
replace_bundle_example_yaml