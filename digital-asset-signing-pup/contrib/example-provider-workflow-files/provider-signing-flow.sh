#!/usr/bin/env bash
set -euo pipefail

echo "1) Build artifact"
echo "2) Compute artifact digest"
echo "3) Produce manifest-v1.json"
echo "4) Sign manifest with provider key"
echo "5) Produce DSSE/in-toto provenance bundle"
echo "6) Submit POST /artifact/verify and POST /artifact/publish"
