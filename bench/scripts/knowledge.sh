#!/usr/bin/env bash
# knowledge-001 (docs/research/knowledge-001-preregistration.md): every
# variant, live, 3 repeats, then the judge. Keys from the environment.
set -euo pipefail
cd "$(dirname "$0")/../.."
TESSERA=${TESSERA:-./bin/tessera}
OUT=bench/results/knowledge-001
for v in none kb kb-check; do
  mkdir -p "$OUT/$v"
  (for r in 1 2 3; do
    [ -s "$OUT/$v/r$r.json" ] && continue
    "$TESSERA" test "bench/knowledge/$v" --decide typesafe:jev-1.13.0 --generate openrouter:deepseek/deepseek-v4.1-flash \
      --max-cost-usd 0.3 --out "$OUT/$v/r$r.json" -v > "$OUT/$v/r$r.log" 2>&1 || true
  done) &
done
wait
python3 bench/knowledge/judge.py "$OUT"/*/r*.json --out "$OUT/judged.jsonl" --max-cost 1.5
