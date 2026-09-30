#!/usr/bin/env bash
# Blind naturalness judge (docs/research/rfc0004-baseline-preregistration.md):
# Tessera's C0 transcripts from a market run against the competitors'
# transcripts of an earlier run on the same tasks, both orders.
#
#   bench/scripts/naturalness.sh -tessera bench/results/NAME/runs.jsonl \
#     -competitors bench/results/market-003/runs.jsonl -tasks bench/market/tasks-003.json \
#     -out bench/results/NAME/naturalness [-max-cost 2.5]
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
: "${OPENROUTER_API_KEY:?set OPENROUTER_API_KEY}"
tess= comp= tasks= out= maxcost=2.5
while [ $# -gt 0 ]; do case "$1" in
  -tessera) tess=$2;; -competitors) comp=$2;; -tasks) tasks=$2;; -out) out=$2;; -max-cost) maxcost=$2;;
  *) echo "unknown flag $1" >&2; exit 2;; esac; shift 2; done
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
python3 - "$tess" "$comp" "$tmp/pairs.jsonl" <<'PY'
import json, sys
tess, comp, out = sys.argv[1:]
c = [l for l in open(comp) if json.loads(l)["competitor"] != "tessera"]
t = [l for l in open(tess) if json.loads(l)["competitor"] == "tessera" and json.loads(l)["condition"] == "C0"]
open(out, "w").write("".join(c + t))
PY
python3 bench/market/naturalness/judge.py "$tmp/pairs.jsonl" "$tasks" "$out" --max-cost "$maxcost"
