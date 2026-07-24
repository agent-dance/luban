#!/bin/bash
set -euo pipefail
cd /workspace
git apply /tests/gold.patch
