#!/usr/bin/env bash
# knowledge-002 (docs/research/knowledge-002-preregistration.md).
#   bench/scripts/relevance.sh <split> <label>=<binary>:<agent-dir> ...
# e.g. relevance.sh dev base=/tmp/tessera-kb-base:rel-off r1=/tmp/tessera-kb-base:rel-on
set -euo pipefail
cd "$(dirname "$0")/../.."
split=$1; shift
OUT=bench/results/knowledge-002
for spec in "$@"; do
  label=${spec%%=*}; rest=${spec#*=}; bin=${rest%%:*}; dir=${rest#*:}
  mkdir -p "$OUT/$label"
  for r in 1 2 3; do
    [ -s "$OUT/$label/$split-r$r.json" ] && continue
    "$bin" test "bench/relevance/$dir" --only "$split/" --decide typesafe:jev-1.13.0 --generate openrouter:deepseek/deepseek-v4.1-flash \
      --max-cost-usd 0.3 --out "$OUT/$label/$split-r$r.json" -v > "$OUT/$label/$split-r$r.log" 2>&1 &
  done
done
wait
