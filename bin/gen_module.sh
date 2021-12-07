#!/bin/bash

# usage example: bash gen_module.sh my_module
# get $MPATH/src/my_module

MPATH=${MPATH:-$GOPATH}
echo "Module path is $MPATH"
# get module name
if [[ "$#" -eq 0 ]]; then
  echo "No specified module name. Please provide one and try again."
  exit 1
else
  module_name=$1
  Module_name="${module_name^}"
fi

# check destination directory
if [ -d "$MPATH/src/$module_name" ]; then
  read -p "$MPATH/src/$module_name already exists. Delete? (yes/no)" delete
  if [[ "$delete" != "yes" ]]; then
    echo "Exit without actions."
    exit 1
  else
    echo "Delete old $MPATH/src/$module_name directory."
    rm -rf "$MPATH/src/$module_name"
  fi
fi

# generate files from example
cp -r "$MPATH/src/slime/modules/example" "$MPATH/src/$module_name"
cd "$MPATH/src/$module_name"

function replace_example() {
  sed -i "s/example/$module_name/g" $1
  sed -i "s/Example/$Module_name/g" $1
  echo "replace $1"
}

# replace go.mod
replace_example go.mod

# replace api
replace_example api/v1alpha1/config.proto
cd api/v1alpha1
GO111MODULE="off" protoc -I=. -I=$GOPATH/src -I=$GOPATH/src/github.com/gogo/protobuf/protobuf --gogo_opt=paths=source_relative --gogo_out=. config.proto
cd ../..

# replace main.go
replace_example main.go

# replace controllers
replace_example controllers/example_reconciler.go
mv controllers/example_reconciler.go "controllers/${module_name}_reconciler.go"

# replace model
replace_example model/model.go

# replace module
replace_example module/module.go

# replace install
replace_example install/slimeboot_example.yaml
mv install/slimeboot_example.yaml install/slimeboot_${module_name}.yaml

# replace publish.sh
replace_example publish.sh
sed -i "s/..\/..\/bin\/publish.sh/..\/slime\/bin\/publish.sh/" publish.sh

echo "module $module_name generates successfully"