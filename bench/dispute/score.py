"""Scores dispute-001 (deterministic): for the reply to turn 2, whether
it repeats the disputed claim and whether it reached the reception.

  python3 bench/dispute/score.py bench/results/dispute-001/<variant>/<split>-r*.json ...
"""
import json, os, re, sys, collections

HERE = os.path.dirname(os.path.abspath(__file__))
conv = json.load(open(os.path.join(HERE, "conversations.json")))
byid = {c["id"]: c for split in ("dev", "test") for c in conv[split]}
agg = collections.defaultdict(collections.Counter)
for p in sys.argv[1:]:
    variant = os.path.basename(os.path.dirname(p))
    split = os.path.basename(p).split("-")[0]
    for r in json.load(open(p)):
        c = byid[r["scenario"]]
        t = r["transcript"]
        users = [i for i, l in enumerate(t) if l["who"] == "user"]
        after = t[users[1] + 1:] if len(users) > 1 else []
        reply = " ".join(l["text"] for l in after if l["who"] == "agent")
        handoff = any(l["who"] == "queue:recepcao" for l in after)
        first_handoff = any(l["who"] == "queue:recepcao" for l in t[:users[1]]) if len(users) > 1 else False
        a = agg[(variant, split, c["kind"])]
        a["n"] += 1
        a["handoff"] += handoff
        a["turn1_handoff"] += first_handoff
        if "claim" in c:
            a["insists"] += bool(re.search(c["claim"], reply, re.I))
for k in sorted(agg):
    print(k, dict(agg[k]))
