#!/usr/bin/env bash
# Runs the market benchmark (bench/market) end to end: the tool server,
# the Python competitors when asked, the harness. Keys come from the
# environment (OPENROUTER_API_KEY, TYPESAFE_API_KEY); nothing is printed.
#
#   bench/scripts/market.sh -tasks bench/market/tasks-003.json -out bench/results/NAME \
#     [-competitors tessera] [-conditions C0,C1,C2,C3] [-repeats 3] \
#     [-defs bench/market/tessera-warm] [-tessera-model model@provider] [-max-cost 2] \
#     [-only t01,t02]   # task ids; empty: all
#     [-keep-db 1]      # keep the per-run journals (tessera drift reads them)
#
# Competitors: tessera, plain, langgraph, langgraph-sqlite, agents-sdk,
# agents-sdk-sqlite, crewai (the Python ones need bench/market/.venv, and
# crewai bench/market/.venv-crewai: requirements-crewai.txt; see
# bench/market/competitors/requirements.txt).
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
: "${OPENROUTER_API_KEY:?set OPENROUTER_API_KEY}" "${TYPESAFE_API_KEY:?set TYPESAFE_API_KEY}"
tasks= out= competitors=tessera conditions=C0,C1,C2,C3 repeats=3 defs=bench/market/tessera-warm tmodel= maxcost=2 only= keepdb=
while [ $# -gt 0 ]; do case "$1" in
  -tasks) tasks=$2;; -out) out=$2;; -competitors) competitors=$2;; -conditions) conditions=$2;;
  -repeats) repeats=$2;; -defs) defs=$2;; -tessera-model) tmodel=$2;; -max-cost) maxcost=$2;; -only) only=$2;; -keep-db) keepdb=$2;;
  *) echo "unknown flag $1" >&2; exit 2;; esac; shift 2; done
[ -n "$tasks" ] && [ -n "$out" ] || { echo "need -tasks and -out" >&2; exit 2; }
tmp=$(mktemp -d); pids=()
cleanup() { for p in "${pids[@]:-}"; do kill "$p" 2>/dev/null || true; wait "$p" 2>/dev/null || true; done; rm -rf "$tmp"; }
trap cleanup EXIT
go build -o "$tmp/toolserver" ./bench/market/toolserver
go build -o "$tmp/marketbench" ./bench/market/marketbench
"$tmp/toolserver" -tasks "$tasks" -addr 127.0.0.1:8930 -tls-addr 127.0.0.1:8931 -cert-out "$tmp/cert.pem" >"$tmp/toolserver.log" 2>&1 & pids+=($!)
ext=""; port=8940
for fw in langgraph langgraph-sqlite agents-sdk agents-sdk-sqlite crewai; do
  case ",$competitors," in *",$fw,"*)
    mkdir -p "$tmp/pydata"
    venv=../.venv; [ "$fw" = crewai ] && venv=../.venv-crewai  # CrewAI: Python < 3.14, openai < 3 (market-bench-005)
    (cd bench/market/competitors && OTEL_SDK_DISABLED=true CREWAI_DISABLE_TELEMETRY=true CREWAI_DISABLE_TRACKING=true BENCH_TOOLSERVER=http://127.0.0.1:8930 exec $venv/bin/python server.py --framework "$fw" --port $port --data "$tmp/pydata" >"$tmp/$fw.log" 2>&1) & pids+=($!)
    ext="$ext${ext:+,}$fw=http://127.0.0.1:$port"; port=$((port+1));;
  esac
done
sleep 8
args=(-tasks-file "$tasks" -toolserver http://127.0.0.1:8930 -tls-addr 127.0.0.1:8931 -cert "$tmp/cert.pem"
  -tessera-defs "$defs" -competitors "$competitors" -conditions "$conditions" -repeats "$repeats"
  -max-cost-usd "$maxcost" -out "$out" -parallel 12)
[ -n "$ext" ] && args+=(-external "$ext")
[ -n "$tmodel" ] && args+=(-tessera-model "$tmodel")
[ -n "$only" ] && args+=(-tasks "$only")
"$tmp/marketbench" "${args[@]}"
# The per-run databases are large and not committed.
[ -n "$keepdb" ] || rm -rf "$out/db"
