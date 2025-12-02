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

print_help() {
    echo "Usage: entrypoint.sh [options]"
    echo
    echo "Options:"
    echo "-h    Show this help message"
    echo "Note: Environment is auto-detected from UBUNTU_LIBDIR variable"
    echo "      UBUNTU_LIBDIR=RHEL9 for Kubernetes deployments"
    echo "      UBUNTU_LIBDIR=UBUNTU22/UBUNTU24 for Debian deployments"
}

while getopts "h" option; do
    case $option in
        h)
            print_help
            exit 0 ;;
        \?)
            echo "Invalid option"
            exit 1 ;;
    esac
done

if [ -d "/device-config-manager" ]; then
    echo "Sucessfully mounted DCM directory!"
else
    echo "Mounting DCM directory unsuccessful!"
fi

cd /device-config-manager
TOP_DIR=$(pwd)
echo "Current directory: $(pwd)"
rm -rf $TOP_DIR/bin/device-config-manager*-$UBUNTU_VERSION

mkdir -p $TOP_DIR/build/assets/
mkdir -p $TOP_DIR/bin

# Auto-detect environment from UBUNTU_LIBDIR and copy appropriate assets
echo "Copying assets from path $TOP_DIR/assets/amd_smi_lib/x86_64/$UBUNTU_LIBDIR/lib/"
cp -r $TOP_DIR/assets/amd_smi_lib/x86_64/$UBUNTU_LIBDIR/lib/* $TOP_DIR/build/assets

echo "Building Unified DCM binary for $UBUNTU_VERSION"
go build -ldflags "-s -w -X main.Version=$VERSION -X main.GitCommit=$GIT_COMMIT -X main.BuildDate=$BUILD_DATE " -o dcm_build_unified $TOP_DIR/cmd/deviceconfigmanager/main.go
cp dcm_build_unified $TOP_DIR/bin/device-config-manager-$UBUNTU_VERSION
rm -rf dcm_build_unified $TOP_DIR/build/

if [ $? -ne 0 ]; then
echo "Unified DCM build failed. Exiting..."
exit 1
fi

echo "Successfully built Unified DCM binary for $UBUNTU_VERSION"
