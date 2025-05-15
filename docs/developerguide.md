# Developer Guide

This document provides build instructions and guidance for developers working on the AMD Device Config Manager repository.

## Git submodule setup

Make sure to update the submodules on every pull from the repository.
```bash
git submodule update --init --recursive
```

## Environment Setup

The project Makefile provides a easy way to create a docker build container that packages the Docker and Go versions needed to build this repository. The following environment variables can be set, either directly or via a `dev.env` file:

- `DOCKER_REGISTRY`: Docker registry (default: `docker.io/rocm`).
- `DOCKER_BUILDER_TAG`: Docker build container tag (default: `v1.0`).
- `BUILD_BASE_IMAGE`: Base image for Docker build container (default: `ubuntu:22.04`).
- `UBUNTU_VERSION`: Ubuntu version for builds (`jammy` for 22.04, `noble` for 24.04).

## Build Prerequisites

Before starting, ensure you have Docker installed and running with the user permissions set appropriately.

## Quick Start

To quickly build everything using Docker:
```bash
make default
```

The default target creates a docker build container that packages the developer tools required to build all other targets in the Makefile and builds the `all` target in this build container.

## Building Components

### Build and Launch Docker Build Container Shell

Run the following command to start a Docker-based build container shell:

```bash
make docker-shell
```

This gives you an interactive Docker environment with necessary tools pre-installed. It is recommended to run all other Makefile targets in this build environment.

### Compiling the AMD Device Config Manager

To compile from within the build environment, run:

```bash
make all
```

This command builds:
- AMD Device Config Manager
- Proto-generated code
- AMD Device Config Manager docker

### Building a Debian Package

To build a Debian package for Ubuntu:

```bash
make pkg
```

This will create `.deb` packages in the `bin` directory.

To run the debian on a MI300 node

```bash
sudo dpkg -i amdgpu-configmanager_22.04_amd64.deb
dpkg -L amdgpu-configmanager

sudo systemctl start amd-config-manager.service
sudo journalctl -fu amd-config-manager.service
```

To restart the systemd service file

```bash
sudo systemctl start amd-config-manager.service
```

To stop the systemd service file

```bash
sudo systemctl stop amd-config-manager.service
```

To remove the debian package from the node

```bash
sudo dpkg --configure -a
sudo dpkg --purge --force-remove-reinstreq amdgpu-configmanager
dpkg -l | grep amdgpu-configmanager
rm amdgpu-configmanager_22.04_amd64.deb
```

### Build Docker images

Build standard dcm image:

```bash
make dcm-docker
```

### Helm Chart Packaging

To package Helm charts:

```bash
make helm-install
```

## Build AMD SMI
This is a built out of [AMD SMI Lib](git@github.com:ROCm/amdsmi.git), to
access AMD GPU hardware driver

#### Build Container (one time)
```bash
make amdsmi-build
```

#### Compile AMDSMI
```bash
make amdsmi-compile
```