#!/usr/bin/env bash

set -euo pipefail

if (($# != 1)); then
  echo "usage: $0 <message-or-path>" >&2
  exit 2
fi

input="$1"
if [[ -f "$input" ]]; then
  message="$(head -n 1 "$input")"
else
  message="$input"
fi

pattern='^(feat|fix|docs|refactor|test|chore)(\([A-Za-z0-9._/-]+\))?!?: .+'

if [[ ! "$message" =~ $pattern ]]; then
  echo "FAIL: commit message must follow Conventional Commits" >&2
  echo "example: feat(dashboard): 네임스페이스 생성 API 추가" >&2
  exit 1
fi

subject="${message#*: }"
if ! perl -CSDA -e 'exit(($ARGV[0] =~ /\p{Hangul}/) ? 0 : 1)' "$subject"; then
  echo "FAIL: commit summary must be written in Korean" >&2
  echo "example: feat(harness): 로컬 개발 하네스 추가" >&2
  exit 1
fi
