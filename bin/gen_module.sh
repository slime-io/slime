#!/bin/bash

# usage example: bash gen_module.sh my_module
# get $MPATH/src/my_module

BASEDIR=$(dirname $0)
TEMPLATE_PATH=${BASEDIR}/../modules/example
MPATH=$(realpath ${MPATH:-"${BASEDIR}/../staging/src/slime.io/slime/modules"})
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
if [ -d "$MPATH/$module_name" ]; then
  read -p "$MPATH/$module_name already exists. Delete? (yes/no)" delete
  if [[ "$delete" != "yes" ]]; then
    echo "Exit without actions."
    exit 1
  else
    echo "Delete old $MPATH/$module_name directory."
    rm -rf "$MPATH/$module_name"
  fi
fi

# generate files from example
cp -r "${TEMPLATE_PATH}" "$MPATH/$module_name"
cd "$MPATH/$module_name"

function replace_example() {
  sed -i "s/example/$module_name/g" $1
  sed -i "s/Example/$Module_name/g" $1
}

function rename_file(){
  src=$1
  if grep "example" $src;then
    dst="${src/example/"${module_name}"}"
    mv $src $dst
  fi
}

for f in $(find . -type f);do
  replace_example $f
  rename_file $f
done

# modify go.mod
sed -i "/need delete/d" go.mod
sed -i "/need uncomment/s/\/\/ //" go.mod

echo "module $module_name generates successfully"
