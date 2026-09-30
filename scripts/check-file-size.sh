#!/usr/bin/env bash
# Fails if any Go file exceeds the line limit (AGENTS.md: no file over 800 lines).
# The legacy platform engine grew a 10,684-line file; this keeps that from repeating.
set -euo pipefail
limit="${1:-800}"
status=0
while IFS= read -r -d '' f; do
  n=$(wc -l < "$f")
  if [ "$n" -gt "$limit" ]; then
    echo "$f: $n lines (limit $limit)"
    status=1
  fi
done < <(git -c core.quotePath=off ls-files -z --cached --others --exclude-standard '*.go')
exit $status
