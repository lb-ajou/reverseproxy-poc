#!/usr/bin/env bash

set -euo pipefail

if (($# != 1)); then
  echo "usage: $0 \"type(scope): summary\"" >&2
  exit 2
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

message="$1"

scripts/validate-commit-msg.sh "$message"

if git diff --cached --quiet --ignore-submodules --; then
  echo "FAIL: no staged changes to commit" >&2
  exit 1
fi

scripts/agent-check.sh fast
git commit -m "$message"
