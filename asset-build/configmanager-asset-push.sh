#!/bin/bash

if [ -z $RELEASE ]
then
  echo "RELEASE is not set, return"
  exit 0
fi

echo "Copying device-config-manager artifacts..."

setup_dir () {
    ls -al /device-config-manager/
    BUNDLE_DIR=/device-config-manager/output/
    mkdir -p $BUNDLE_DIR
}

copy_artifacts () {
    # copy amd-config-manager binary
    cp /device-config-manager/bin/amd-config-manager $BUNDLE_DIR/amd-config-manager.gobin
    # copy docker image
    cp /device-config-manager/docker/obj/configmanager-release-*.tgz  $BUNDLE_DIR/
    # list the artifacts copied out
    ls -la $BUNDLE_DIR
}

setup () {
    setup_dir
    copy_artifacts
}

upload () {
    cd $BUNDLE_DIR
    find . -type f -print0 | while IFS= read -r -d $'\0' file;
      do asset-push builds hourly-device-config-manager $RELEASE "$file" ;
      if [ $? -ne 0 ]; then
        exit 1
      fi
    done
}

main () {
  setup
  upload
}

main
exit 0