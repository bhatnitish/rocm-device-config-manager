---
name: sec-fix
description: Use when fixing CVE vulnerabilities reported by trivy in the Device Config Manager (DCM) container image (the `server` Go binary and the redhat OS packages). Provide a CI scan log URL or a built image name. Handles the Go toolchain bump, build-container image update, vendor regeneration, OS-package update, image rebuild, and canonical `trivy image` verification.
agent: sec-fix-agent
---

# sec-fix — CVE Remediation for the DCM Image

Orchestrates end-to-end CVE remediation for the Device Config Manager container image.
Delegates all execution to the `sec-fix-agent`.

## What this skill does

- Scans via the canonical method (`trivy image`, covering BOTH the `server` gobinary and the
  redhat OS packages) using `.claude/bin/scan-image-local.sh`, or parses a CI scan log URL with
  `.claude/bin/parse-trivy-gobinary.sh`. Groups findings by severity and target.
- For gobinary `stdlib` HIGH/CRITICAL: determines the minimum Go toolchain version that closes
  them (checking go.dev for the latest patch) and presents a fix plan for user confirmation.
- For redhat OS-package HIGH/CRITICAL: adds `microdnf update -y` to `docker/Dockerfile`.
- Updates the Go version atomically across `go.mod`, `tools/base-image/Dockerfile`,
  `cmd/deviceconfigmanager/Dockerfile{,.standalone}` and the `Makefile` `mod:` pin; bumps
  `DOCKER_BUILDER_TAG` in `dev.env` + `box.rb`.
- Regenerates `vendor/` via `go mod tidy && go mod vendor` (in a `golang:1.25` container),
  rebuilds the cgo binary + release image, and re-scans to confirm 0 HIGH/CRITICAL.
- **Does NOT add dependency `replace` directives** for go.mod-graph CVEs that are not actually
  compiled into the binary — they fix nothing in the canonical `trivy image` scan.

The agent (`.claude/agents/sec-fix-agent.md`) carries the full build + scan reference.
