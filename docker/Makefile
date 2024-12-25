DOCKER_REGISTRY ?= registry.test.pensando.io:5000/device-config-manager
IMAGE_NAME ?= dcm
IMAGE_TAG_BASE ?= $(DOCKER_REGISTRY)/$(IMAGE_NAME)
# This is the default tag of all images made by this Makefile.
IMAGE_TAG ?= dev
# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):$(IMAGE_TAG)

# name used for saving the container images as tar.gz
DOCKER_CONTAINER_IMG = $(IMAGE_NAME)-$(IMAGE_TAG)
HOURLY_TAG_LABEL ?= latest

.PHONY: docker
docker:
	@echo "building amd config manager docker container"
	@TOP_DIR=$(TOP_DIR) ./build_config_manager_release_docker.sh
