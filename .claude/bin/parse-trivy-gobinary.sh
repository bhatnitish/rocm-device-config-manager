#!/usr/bin/env bash
# parse-trivy-gobinary.sh — extract CVE findings from trivy gobinary sections in a CI scan log.
#
# Usage: parse-trivy-gobinary.sh <URL>
#
# Fetches the log at URL, parses gobinary sections for the DCM binary (server),
# and emits a JSON array of CVE findings to stdout.
# Exits 0 (with []) when no findings; exits 1 on usage error.

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "Usage: $(basename "$0") <CI-scan-log-URL>" >&2
    exit 1
fi

URL="$1"

# Use -c so python reads the script from the argument string; stdin stays on the pipe.
curl -s --fail --compressed "$URL" | python3 -c "
import sys
import json
import re

# DCM ships a single Go binary, installed in the image as home/amd/bin/server.
BINARIES = {'server', 'device-config-manager'}

def parse_trivy(text):
    findings = []
    current_binary = None
    in_table = False

    last_library = ''
    last_severity = ''
    last_installed = ''

    lines = text.splitlines()
    i = 0
    while i < len(lines):
        line = lines[i]

        # Detect any trivy section header: a line whose stripped form ends with (...type...)
        if re.search(r'\(\w[\w-]*\)\s*$', line.rstrip()):
            if line.rstrip().endswith('(gobinary)'):
                header = line.strip()
                path_part = header[:header.rfind('(gobinary)')].strip()
                basename = path_part.rstrip('/').split('/')[-1]

                if basename in BINARIES:
                    current_binary = basename
                else:
                    current_binary = None
            else:
                current_binary = None
            in_table = False
            last_library = ''
            last_severity = ''
            last_installed = ''
            i += 1
            continue

        if current_binary is None:
            i += 1
            continue

        if '|' in line and 'Library' in line and 'Vulnerability' in line:
            in_table = True
            i += 1
            continue

        if re.match(r'^\s*\+[-+]+\+', line):
            i += 1
            continue

        if in_table and line.startswith('|'):
            parts = [p.strip() for p in line.split('|')]
            if len(parts) < 7:
                i += 1
                continue

            library   = parts[1]
            cve       = parts[2]
            severity  = parts[3]
            installed = parts[5]
            fixed     = parts[6]

            if library == 'Library' or cve == 'Vulnerability':
                i += 1
                continue

            if library:
                last_library = library
            else:
                library = last_library

            if severity:
                last_severity = severity
            else:
                severity = last_severity

            if installed:
                last_installed = installed
            else:
                installed = last_installed

            if cve and (cve.startswith('CVE-') or cve.startswith('GHSA-')):
                findings.append({
                    'binary':    current_binary,
                    'library':   library,
                    'cve':       cve,
                    'severity':  severity,
                    'installed': installed,
                    'fixed':     fixed,
                })

            i += 1
            continue

        i += 1

    return findings

data = sys.stdin.read()
findings = parse_trivy(data)
print(json.dumps(findings, indent=2))
"
