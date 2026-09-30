"""Show every candidate's measured outcomes; no automatic post-hoc exclusions."""
import json
from pathlib import Path
import statistics
import sys

p=Path(sys.argv[1])
runs=[json.loads(l) for l in (p/'runs.jsonl').read_text().splitlines()]
judgments={(r['case'],r['variant'],r['repeat']):r['judge'] for r in map(json.loads,(p/'judgments.jsonl').read_text().splitlines())}
summary={}
for v in sorted({r['variant'] for r in runs}):
    rr=[r for r in runs if r['variant']==v]
    jj=[judgments[(r['case'],r['variant'],r['repeat'])] for r in rr]
    lat=sorted(r['latencyMs'] for r in rr)
    summary[v]={'n':len(rr),'unsupported':sum(j['unsupported_claim'] for j in jj),
                'invented':sum(j['invented_fact'] for j in jj),'lostContent':sum(not j['preserved'] for j in jj),
                'natural':statistics.mean(j['natural'] for j in jj),'clear':statistics.mean(j['clear'] for j in jj),
                'fallbacks':sum(bool(r.get('fallback')) for r in rr),'errors':sum(bool(r.get('error')) for r in rr),
                'meanCostUSD':statistics.mean(r['costUSD'] for r in rr),'p50ms':statistics.median(lat),'p95ms':lat[int(.95*(len(lat)-1))]}
print(json.dumps(summary,indent=2))
