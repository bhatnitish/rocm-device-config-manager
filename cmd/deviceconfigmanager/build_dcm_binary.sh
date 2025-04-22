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

DCM_BUILD_DOCKER_IMG="registry.test.pensando.io:5000/dcm-build"

mkdir -p $TOP_DIR/build/assets/
ln -s ../../../device-config-manager device-config-manager

# Check if the image exists locally or in the registry
if ! docker image inspect $DCM_BUILD_DOCKER_IMG:$UBUNTU_VERSION > /dev/null 2>&1; then
    echo "Image not found locally. Checking in registry..."
    # if ! docker pull $DCM_BUILD_DOCKER_IMG:$UBUNTU_VERSION; then
    echo "Image not found in registry. Building the image..."
    # Build the image if it's not found locally or in the registry
    if [ "$UBUNTU_VERSION" = "jammy" ]; then
        cp -r $TOP_DIR/assets/amd_smi_lib/x86_64/$UBUNTU_LIBDIR/lib/* $TOP_DIR/build/assets
        docker build --build-arg DCM_BASE_IMAGE=22.04 -t $DCM_BUILD_DOCKER_IMG:jammy -f Dockerfile ../../..
    elif [ "$UBUNTU_VERSION" = "noble" ]; then
        cp -r $TOP_DIR/assets/amd_smi_lib/x86_64/$UBUNTU_LIBDIR/lib/* $TOP_DIR/build/assets
        docker build --build-arg DCM_BASE_IMAGE=24.04 -t $DCM_BUILD_DOCKER_IMG:noble -f Dockerfile ../../..
    fi
    # Push the newly built image to the registry
    docker push $DCM_BUILD_DOCKER_IMG:$UBUNTU_VERSION
    # fi
else
    echo "Image found locally or in the registry. Using the existing image..."
fi

rm -rf device-config-manager
docker run -it --name dcm_build_container $DCM_BUILD_DOCKER_IMG:$UBUNTU_VERSION  bash -c "
  go build -o dcm_build /device-config-manager/cmd/deviceconfigmanager/main.go
"

mkdir -p $TOP_DIR/bin

docker cp dcm_build_container:/device-config-manager/dcm_build $TOP_DIR/bin/device-config-manager-$UBUNTU_VERSION
docker stop dcm_build_container
docker rm dcm_build_container

rm -rf $TOP_DIR/build/

exit 0