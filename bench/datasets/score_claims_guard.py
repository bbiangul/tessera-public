"""Scores guard-001 trials at each threshold: false positives on
not-claims and misses on claims (a trial is a claim when P(true) >= t).

  python3 bench/datasets/score_claims_guard.py <trials.jsonl>... [--show]
"""
import json, sys

paths = [a for a in sys.argv[1:] if a.endswith(".jsonl")]
for p in paths:
    rows = [json.loads(l) for l in open(p)]
    print(p)
    for t in (0.5, 0.6, 0.7, 0.8):
        fp = sum(1 for r in rows if r["gold"] == "false" and r["probs"].get("true", 0) >= t)
        miss = sum(1 for r in rows if r["gold"] == "true" and r["probs"].get("true", 0) < t)
        n0 = sum(1 for r in rows if r["gold"] == "false"); n1 = len(rows) - n0
        print(f"  t={t}: false positives {fp}/{n0}, missed claims {miss}/{n1}")
    if "--show" in sys.argv:
        for r in rows:
            if r["repeat"] == 0:
                print(f"    {r['caseId']} gold={r['gold']} p={r['probs'].get('true', 0):.2f}")
