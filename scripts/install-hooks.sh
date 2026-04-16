#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

chmod +x .githooks/pre-commit .githooks/commit-msg \
  scripts/agent-check.sh scripts/agent-commit.sh \
  scripts/install-hooks.sh scripts/validate-commit-msg.sh

git config core.hooksPath .githooks
echo "Installed repository hooks from .githooks"
