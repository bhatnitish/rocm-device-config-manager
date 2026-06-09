#!/usr/bin/env bash
# scan-image-local.sh — canonical local CVE scan of the built DCM image using trivy.
#
# This mirrors the company scan report method (trivy image), which covers BOTH the
# Go gobinary build-info AND the redhat OS packages — unlike `trivy fs`, which only
# scans the go.mod graph and misses OS packages.
#
# Usage: scan-image-local.sh <image[:tag]>
#        scan-image-local.sh dcm-rocm-7.13:cve
#
# Output: trivy table output to stdout (HIGH/CRITICAL only)
# Exit code: 1 if any HIGH or CRITICAL findings exist; 0 otherwise

set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <image[:tag]>" >&2
    exit 1
fi

IMAGE="$1"

run_trivy() {
    if command -v trivy &>/dev/null; then
        trivy image "$IMAGE" --ignore-unfixed --severity HIGH,CRITICAL \
            -f table --scanners vuln --exit-code 1
    else
        # No host trivy — use the official container, mounting the docker socket
        # so it can read locally-built images.
        docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
            aquasec/trivy:latest image "$IMAGE" --ignore-unfixed \
            --severity HIGH,CRITICAL -f table --scanners vuln --exit-code 1
    fi
}

run_trivy
