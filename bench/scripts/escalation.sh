#!/usr/bin/env bash
# Escalation benchmark (bench/escalation, issue #25): the LLM competitors,
# today's routed Tessera and the free-mode handoff on one question set,
# then the blind judge on repeats 1-5.
#
#   bench/scripts/escalation.sh -set test4 -out bench/results/NAME [-repeats 20] \
#     [-stacks plain,langgraph,agents-sdk,tessera,tessera-handoff]
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
: "${OPENROUTER_API_KEY:?set OPENROUTER_API_KEY}" "${TYPESAFE_API_KEY:?set TYPESAFE_API_KEY}"
set_= out= repeats=20 stacks=plain,langgraph,agents-sdk,tessera,tessera-handoff
while [ $# -gt 0 ]; do case "$1" in
  -set) set_=$2;; -out) out=$2;; -repeats) repeats=$2;; -stacks) stacks=$2;;
  *) echo "unknown flag $1" >&2; exit 2;; esac; shift 2; done
mkdir -p "$out"
llm=$(echo "$stacks" | tr ',' '\n' | grep -E '^(plain|langgraph|agents-sdk)$' | paste -sd, - || true)
[ -n "$llm" ] && bench/market/.venv/bin/python bench/escalation/competitors.py --competitors "$llm" --set "$set_" --repeats "$repeats" --out "$out/runs.jsonl" --max-cost 1
for s in tessera tessera-handoff; do
  case ",$stacks," in *",$s,"*)
    go run ./bench/escalation/escbench -defs "bench/escalation/$s" -name "$s" -set "$set_" -repeats "$repeats" -out "$out/runs.jsonl" -max-cost-usd 0.5;;
  esac
done
python3 bench/escalation/judge.py "$out/runs.jsonl" "$out/judgments.jsonl" --repeats 5 --max-cost 3
python3 bench/escalation/report.py "$out/runs.jsonl" "$out/judgments.jsonl"
