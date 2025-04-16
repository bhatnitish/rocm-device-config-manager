#!/bin/bash -e
#
# Copyright(C) Advanced Micro Devices, Inc. All rights reserved.
#
# You may not use this software and documentation (if any) (collectively,
# the "Materials") except in compliance with the terms and conditions of
# the Software License Agreement included with the Materials or otherwise as
# set forth in writing and signed by you and an authorized signatory of AMD.
# If you do not have a copy of the Software License Agreement, contact your
# AMD representative for a copy.
#
# You agree that you will not reverse engineer or decompile the Materials,
# in whole or in part, except as allowed by applicable law.
#
# THE MATERIALS ARE DISTRIBUTED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OR
# REPRESENTATIONS OF ANY KIND, EITHER EXPRESS OR IMPLIED.

# script is used to build the dcm go binary for different OS versions

# create a tmp directory to fill the AMD SMI assets

mkdir -p $TOP_DIR/build/assets/
ln -s ../../../device-config-manager device-config-manager

if [ "$UBUNTU_VERSION" = "jammy" ]; then
    cp -r $TOP_DIR/assets/amd_smi_lib/x86_64/jammy/lib/* $TOP_DIR/build/assets
    docker build -t img -f Dockerfile.ubuntu22 ../../..
elif [ "$UBUNTU_VERSION" = "noble" ]; then
    cp -r $TOP_DIR/assets/amd_smi_lib/x86_64/noble/lib/* $TOP_DIR/build/assets
    docker build -t img -f Dockerfile.ubuntu24 ../../..
fi

rm -rf device-config-manager
docker run -it --name dcm_build_container img:latest  bash -c "
  echo 'Listing files:' &&
  ls -l &&
  echo 'Current working directory:' &&
  pwd &&
  go build -o dcm_build /device-config-manager/cmd/deviceconfigmanager/main.go
"

mkdir -p $TOP_DIR/bin

docker cp dcm_build_container:/device-config-manager/dcm_build $TOP_DIR/bin/device-config-manager-$UBUNTU_VERSION
docker stop dcm_build_container
docker rm dcm_build_container

rm -rf $TOP_DIR/build/

exit 0