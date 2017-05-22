#!/bin/bash

script_root=`dirname "${BASH_SOURCE}"`
source $script_root/../../env.sh

echo "build ffan/rds/tikv image ..."
docker build $DPROXY -t $REGISTRY/ffan/rds/tikv:$VERSION ./