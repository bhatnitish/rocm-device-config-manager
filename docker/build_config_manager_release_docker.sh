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
#
#
# script to generate tarball with config manager docker, entrypoint script
# and docker_run script to create the docker container

VER=v1
SAVE_IMAGE=0
PUBLISH_IMAGE=0
DOCKER_REGISTRY="registry.test.pensando.io:5000/device-config-manager/"
CONFIGMANAGER_IMAGE="conf_manager"

IMAGE_URL="${DOCKER_REGISTRY}${CONFIGMANAGER_IMAGE}:${VER}"

echo $TOP_DIR
cp -r $TOP_DIR/assets/amd_smi_lib $TOP_DIR/docker/smilib
ln -f $TOP_DIR/cmd/deviceconfigmanager/bin/amd-config-manager $TOP_DIR/docker/amd-config-manager

docker build -t $IMAGE_URL . -f Dockerfile && docker save -o configmanager-docker-$VER.tar $IMAGE_URL
if [ $? -eq 0 ]; then
    gzip configmanager-docker-$VER.tar
    mv configmanager-docker-$VER.tar.gz configmanager-docker-$VER.tgz
else
    echo "Failed to build docker image"
    exit $?
fi
rm -rf smilib

exit 0
