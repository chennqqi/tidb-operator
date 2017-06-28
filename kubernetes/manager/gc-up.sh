#!/bin/bash

set -e

script_root=`dirname "${BASH_SOURCE}"`
source $script_root/../env.sh

version=latest

 echo "Creating tidb gc daemonset..."
sed_script=""
for var in version; do
  sed_script+="s,{{$var}},${!var},g;"
done
cat gc-daemonset.yaml | sed -e "$sed_script" | $KUBECTL $KUBECTL_OPTIONS create -f -