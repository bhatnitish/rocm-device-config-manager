# Build System

## Quick Reference

```bash
make dcm-binary              # Build K8s binary
make dcm-binary ENV=debian   # Build Debian binary
make dcm-docker              # Build Docker image
make all                     # Binary + Docker image
make fmt                     # go fmt
make vet                     # go vet
make lint                    # golangci-lint v1.53.1 (5m timeout)
make gen                     # Generate all protobuf code
make e2e                     # Run E2E tests (requires K8s cluster)
make docker-shell            # Interactive dev shell in Docker
```

## CGo Compilation

**CGo is required** — the GPU manager uses C bindings to AMD SMI library.

```c
#cgo CFLAGS: -I/device-config-manager/build/assets/amd_smi
#cgo LDFLAGS: -L/device-config-manager/build/assets -lamd_smi -ldrm_amdgpu -ldrm
#include "/device-config-manager/build/assets/amdsmi.h"
```

**Environment variables** (set in Dockerfiles/entrypoints):
- `CGO_ENABLED=1`
- `GOOS=linux`
- `GOARCH=amd64`
- `GOFLAGS=-mod=vendor`

**Runtime library loading** (Docker entrypoint):
```bash
LD_PRELOAD=/home/amd/lib/libamd_smi.so.26 /home/amd/bin/server
```

**Debian systemd service**:
```
Environment="LD_LIBRARY_PATH=/usr/local/configs/lib:$LD_LIBRARY_PATH"
```

**Binary ldflags**:
```
-s -w -X main.Version=$VERSION -X main.GitCommit=$GIT_COMMIT -X main.BuildDate=$BUILD_DATE
```

## AMD SMI Library Compilation

Pre-compiled libraries live in `assets/amd_smi_lib/x86_64/<PLATFORM>/lib/`. Platform-specific builder images compile from source.

### Platform Library Directories

```
assets/amd_smi_lib/x86_64/
├── RHEL9/lib/
├── UBUNTU22/lib/
├── UBUNTU24/lib/
└── AZURE3/lib/
```

### Builder Images

| Platform | Dockerfile | Base Image | Builder Image Tag |
|----------|-----------|------------|------------------|
| RHEL 9 | `tools/smilib-builderimage/Dockerfile.rhel9.4` | `registry.access.redhat.com/ubi9/ubi:9.4` | `amdsmi-builder-dcm:rhel9` |
| Ubuntu 22.04 | `tools/smilib-builderimage/Dockerfile.ubuntu22` | `ubuntu:22.04` | `amdsmi-builder-dcm:ub22` |
| Ubuntu 24.04 | `tools/smilib-builderimage/Dockerfile.ubuntu24` | `ubuntu:24.04` | `amdsmi-builder-dcm:ub24` |
| Azure Linux 3 | `tools/smilib-builderimage/Dockerfile.azure` | `mcr.microsoft.com/azurelinux/base/core:3.0` | `amdsmi-builder-dcm:azure` |

### Compilation Entrypoint (`tools/smilib-builderimage/entrypoint.sh`)

1. Git checkout to `AMDSMI_BRANCH` (default: `rocm-7.2.1`)
2. Git reset to `AMDSMI_COMMIT` (default: `1e91f3c1527617066f50c22f9ec4368fe82e1a3c`)
3. CMake with `-DCMAKE_C_COMPILER=gcc -DCMAKE_CXX_COMPILER=g++ -DENABLE_ESMI_LIB=OFF`
4. `make -j $(nproc)` then `make install`
5. Copies to `build/dcmout/`:
   - `libamd_smi.so*` (versioned shared libraries)
   - `amdsmi.h` from `/opt/rocm/include/amd_smi/`
   - `libdrm_amdgpu.so*` and `libdrm.so*`
   - Library source paths differ: Ubuntu uses `/usr/lib/x86_64-linux-gnu/`, RHEL/Azure uses `/usr/lib64/`

### AMD SMI Build Targets

```bash
make build-amdsmi-all      # Build all builder images
make compile-amdsmi-all    # Compile libraries for all platforms
make amdsmi-update         # Update from branch/commit and compile all
make amdsmi-build-rhel     # Build RHEL9 builder image
make amdsmi-compile-rhel   # Compile RHEL9 libraries
# Same pattern for ub22, ub24, azure
```

## Docker Images

### Main DCM Image (`docker/Dockerfile`)

- Base: `registry.access.redhat.com/ubi9/ubi-minimal:9.7` (Red Hat UBI Minimal)
- Packages: sudo, procps, libdrm-amdgpu, kmod, dbus, pciutils, jq, and more
- AMD repos: `amdgpu.repo` (radeon.com/graphics/7.2.1) + `rocm.repo` (radeon.com/rocm/rhel9/7.2.1)
- Binary: `/home/amd/bin/server`
- Libraries: `/home/amd/lib/`
- Entrypoint: `/home/amd/tools/entrypoint.sh`

### Build Container (`cmd/deviceconfigmanager/Dockerfile`)

- Base: `ubuntu:22.04` (configurable via `DCM_BASE_IMAGE`)
- Go: `go1.25.8.linux-amd64`
- Entrypoint copies platform-specific SMI libraries and builds the binary

### Dev Build Container (`tools/base-image/Dockerfile`)

- Base: `ubuntu:22.04`
- Full toolchain: Go, Docker CE, kubectl v1.30.4, Helm, Node.js v22.x, Python, cmake, protobuf-compiler
- Doc tools: Sphinx, markdownlint-cli2, pyspelling

## Binary Build Targets

```bash
make dcm-binary            # ENV defaults to k8s → bin/device-config-manager-rhel (RHEL libs)
make dcm-binary ENV=debian # Uses UBUNTU_VERSION → bin/device-config-manager-{jammy|noble}
make dcm                   # Convenience for dcm-binary ENV=k8s
make dcm-st                # Convenience for dcm-binary ENV=debian
```

**K8s builds**: Always use RHEL9 libraries via `copy-assets-k8s` target.
**Debian builds**: Use Ubuntu-specific libraries from `assets/amd_smi_lib/x86_64/${UBUNTU_LIBDIR}/lib/`.

## Debian Package Creation

```bash
make pkg-jammy    # Ubuntu 22.04 package
make pkg-noble    # Ubuntu 24.04 package
```

**Package workflow**:
1. Clean existing `.deb` files
2. Build binary with `ENV=debian`
3. Copy SMI libraries to `debian/usr/local/configs/`
4. Copy binary to `debian/usr/local/bin/`
5. Strip binary (`strip` for size optimization)
6. Template substitution in `debian/DEBIAN/control` and systemd service file
7. `dpkg-deb -Zxz --build debian`
8. Rename to `amdgpu-configmanager_<version>_amd64.deb`

**Package structure**:
- Binary: `/usr/local/bin/device-config-manager-{jammy|noble}`
- Libraries: `/usr/local/configs/lib/`
- systemd service: `/usr/lib/systemd/system/amd-config-manager.service`
- Config: `/etc/config-manager-ainic/ainic_config.json` (conffile)
- Post-install: Sets `LD_LIBRARY_PATH` in `/etc/profile.d/dcm.sh`
- Pre-remove: Stops and disables systemd service

**Version format**: `amdgpu-configmanager_1.0.0~22.04_amd64.deb`

## Helm Chart Build

```bash
make helm-lint     # Validate chart
make helm-build    # Package chart
make helm-deploy   # Package + install
make helm-install  # Install pre-built chart
make helm-uninstall
```

**CI/CD variant** (`make helm`):
- Requires `PROJECT_VERSION` and `DCM_IMAGE_TAG`
- Modifies `values.yaml` with `yq`: sets image repository and tag
- Packages with `--app-version` and `--version`
- Output: `helm-charts-k8s/device-config-manager-charts-<version>.tgz`

## Go Module Management

```bash
make mod    # Full module update
```

1. Ignores `libamdsmi` submodule (creates empty `go.mod` to exclude)
2. `go mod tidy`
3. Sets Go version: `go mod edit -go=1.25.8`
4. CVE fix: Replaces `golang.org/x/oauth2@v0.23.0` → `v0.27.0` (CVE-2025-22868)
5. `go mod vendor`

## Protobuf Generation

```bash
make gen              # All protos
make gen-ainic-proto  # protoc --proto_path=proto --go-grpc_out=. --go_out=. proto/ainic.proto
make gen-gpu-proto    # protoc --proto_path=proto --go-grpc_out=. --go_out=. proto/partition.proto
```

**Tools** (installed via `make gopkglist`):
- `protoc-gen-go@v1.34.2`
- `protoc-gen-go-grpc@v1.5.1`
- `protoc-gen-go-patch` (from alta/protopatch)
- `golangci-lint`
- `goimports`
- `mockgen`

## CI/CD Jobs (`.job.yml`)

| Job | Command | Artifact |
|-----|---------|----------|
| build-all | `make compile-amdsmi-all` | AMD SMI libraries |
| build-debian-package-ub22.04 | `make pkg-jammy` | `bin/amdgpu-configmanager_22.04_amd64.deb` |
| build-debian-package-ub24.04 | `make pkg-noble` | `bin/amdgpu-configmanager_24.04_amd64.deb` |
| build-device-config-manager-docker-ubi9 | `make all` | `docker/obj/config-manager-ubi9-latest.tgz` |
| build-helm-artifact | `make helm-build` | `helm-charts/device-config-manager-charts-v1.4.1.tgz` |
| api-checks | `make checks` | (validation only) |
| docs-sanity | `make docs` | Documentation |
| docs-lint | `make docs-lint` | (validation only) |

## Docker Registries

| Registry | Purpose |
|----------|---------|
| `docker.io/rocm` | Default public registry |
| `registry.test.pensando.io:5000` | Internal test/dev registry |
| `amdpsdo/device-config-manager` | Docker Hub (optional, requires DOCKERHUB_TOKEN) |

## Key Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `DOCKER_REGISTRY` | `docker.io/rocm` | Image registry |
| `DCM_IMAGE_NAME` | `device-config-manager` | Image name |
| `DCM_IMAGE_TAG` | `latest` | Image tag |
| `UBUNTU_VERSION` | `jammy` | Ubuntu codename (jammy/noble) |
| `UBUNTU_LIBDIR` | `UBUNTU22` | Library directory selector |
| `AMDSMI_BRANCH` | `rocm-7.2.1` | AMD SMI source branch |
| `AMDSMI_COMMIT` | `1e91f3c...` | AMD SMI source commit |
| `PROJECT_VERSION` | `1.4.1` | Chart/release version |
| `RELEASE` | (unset) | Release label, sets DEBIAN_VERSION |
| `ENV` | `k8s` | Build mode (k8s/debian) |
