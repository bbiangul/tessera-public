#!/usr/bin/env bash
# self-001 (docs/research/self-001-preregistration.md): every variant,
# live, 3 repeats, then the judge. Keys come from the environment.
set -euo pipefail
cd "$(dirname "$0")/../.."
TESSERA=${TESSERA:-tessera}
OUT=bench/results/self-001
for part in traveler clinic; do
  for dir in bench/self/$part/*/; do
    v=$(basename "$dir")
    mkdir -p "$OUT/$part/$v"
    (for r in 1 2 3; do
      [ -s "$OUT/$part/$v/r$r.json" ] && continue
      "$TESSERA" test "$dir" --decide typesafe:jev-1.13.0 --generate openrouter:deepseek/deepseek-v4.1-flash \
        --max-cost-usd 0.2 --out "$OUT/$part/$v/r$r.json" -v > "$OUT/$part/$v/r$r.log" 2>&1 || true
    done) &
  done
done
wait
python3 bench/self/judge.py "$OUT"/*/*/r*.json --out "$OUT/judged.jsonl" --max-cost 2.5
