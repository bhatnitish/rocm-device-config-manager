# CLAUDE.md — Device Config Manager

Workflow rules and non-obvious gotchas for this repo. Architecture, component deep-dives, and build system details live in [`docs-internal/knowledge/dcm/`](docs-internal/knowledge/dcm/) — load on demand, not every session.

## Build & test

All builds happen inside the build container. Never `go build` against the host toolchain — CGo links against `libamd_smi`, `libdrm_amdgpu`, `libdrm` with hardcoded paths to `/device-config-manager/build/assets/` and will fail without them.

```bash
make docker-shell             # enter build container — required for all builds below
make dcm-binary               # build K8s binary (RHEL9 AMD SMI libs)
make dcm-binary ENV=debian    # build Debian binary
make dcm-docker               # build Docker image (UBI9-based)
make all                      # binary + Docker image
make fmt && make vet          # format + vet
make lint                     # golangci-lint (v1.53.1)
make gen                      # regenerate protobuf code
make e2e                      # E2E tests (requires K8s cluster, 30m timeout)
```

## Repo etiquette

- **Branch off `main`** for all work.
- **Config schema:** GPU config uses `json.Unmarshal`, AINIC config uses `protojson.Unmarshal` — different JSON field name conventions. Edit `.proto` files then `make gen`. Never hand-edit `*.pb.go`.
- **Vendor mode:** dependencies managed via `go mod vendor` (`GOFLAGS=-mod=vendor`).

## Don't touch

- **`*.pb.go`, `gen/`** — generated protobuf code. Regenerate via `make gen`.
- **`vendor/`** — vendored dependencies. Update via `go.mod` + `go mod vendor`.
- **`assets/`, `assets6.3/`** — prebuilt AMD SMI/DRM libraries. Refreshed via dedicated process, not hand-edited.

## Build gotchas

- **In-cluster K8s only**: the K8s client uses `rest.InClusterConfig()` exclusively — no kubeconfig support.
- **File watcher re-add**: after each fsnotify event, the watcher removes and re-adds the file to handle K8s ConfigMap atomic replace. This is intentional, not a bug.
- **No unit tests**: only E2E tests exist (gocheck framework, require a live K8s cluster).

## Where to look

- **Entry point:** [`cmd/deviceconfigmanager/main.go`](cmd/deviceconfigmanager/main.go)
- **GPU manager:** [`pkg/amdgpu/`](pkg/amdgpu/)
- **AINIC manager:** [`pkg/ainic_manager/`](pkg/ainic_manager/)
- **Config manager:** [`pkg/config_manager/`](pkg/config_manager/)
- **K8s utils:** [`pkg/utils/`](pkg/utils/)
- **Shared types/constants:** [`pkg/globals/`](pkg/globals/), [`pkg/interface/`](pkg/interface/)
- **Deep dives:** [`docs-internal/knowledge/dcm/`](docs-internal/knowledge/dcm/)
