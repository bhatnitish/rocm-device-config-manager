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

# ./script.sh -h                # Calls print_help and exits
# ./script.sh -s                # Prints "saving image option set" and sets SAVE_IMAGE=1
# ./script.sh -n my_image       # Sets DOCKER_IMAGE_NAME="my_image"
# ./script.sh -p                # Prints "publish image option set" and sets PUBLISH_IMAGE=1
# ./script.sh -n my_image -s -p # Combines options: sets DOCKER_IMAGE_NAME, SAVE_IMAGE, and PUBLISH_IMAGE

while getopts ":h:sn:p" option; do
    case $option in
        h)
            print_help
            exit ;;
        s)
            echo "saving image option set"
            SAVE_IMAGE=1
            ;;
        n)
            DOCKER_IMAGE_NAME=$OPTARG ;;
        p)
            echo "publish image option set"
            PUBLISH_IMAGE=1
            ;;
        \?)
            echo "Error: Invalid argument"
            exit ;;
    esac
done

IMAGE_DIR=$(pwd)/obj

rm -rf $IMAGE_DIR
mkdir -p $IMAGE_DIR

VER=v1
DOCKER_REGISTRY="registry.test.pensando.io:5000/device-config-manager"
CONFIGMANAGER_IMAGE="conf_manager"

IMAGE_URL="${DOCKER_REGISTRY}:${VER}"

echo $TOP_DIR
cp -r $TOP_DIR/assets/amd_smi_lib $TOP_DIR/docker/smilib
ln -f $TOP_DIR/bin/amd-config-manager $TOP_DIR/docker/amd-config-manager

if [ $PUBLISH_IMAGE == 1 ]; then
    echo "publishing dcm image to $IMAGE_URL"
    docker build -t $IMAGE_URL . -f Dockerfile && docker push $IMAGE_URL
    if [ $? -eq 0 ]; then
        echo "Successfully published image $IMAGE_URL"
    else
        echo "Failed to publish docker image"
        exit $?
    fi
else
    echo "building dcm image to $DOCKER_IMAGE_NAME"
    docker build -t $IMAGE_URL . -f Dockerfile && docker save -o configmanager-docker-$VER.tar $IMAGE_URL
    if [ $? -eq 0 ]; then
        gzip configmanager-docker-$VER.tar
        mv configmanager-docker-$VER.tar.gz configmanager-docker-$VER.tgz
    else
        echo "Failed to build docker image"
        exit $?
    fi
fi

# prepare the final tar ball now
if [ "$SAVE_IMAGE" == 1 ]; then
    echo "Preparing final image ..."
    tar cvzf $IMAGE_DIR/configmanager-release-$VER.tgz configmanager-docker-$VER.tgz
    echo "Image ready in $IMAGE_DIR"
fi

rm -rf smilib

exit 0
