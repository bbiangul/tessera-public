"""agency-001 numbers: the registered hypotheses, from the judged
conversations and the raw results.

  python3 bench/agency/report.py --judged A=judged-A-test.jsonl --judged B=judged-B-test.jsonl \
      --results A=bench/results/agency-001/A-test --results B=bench/results/agency-001/B-test
"""
import argparse, glob, json, os, statistics


def load_judged(path):
    return [json.loads(l) for l in open(path)]


def load_results(d):
    out = []
    for p in sorted(glob.glob(os.path.join(d, "r*", "*.json"))):
        out.extend(json.load(open(p)))
    return out


def stalled(j, kind):
    e = j.get("ended") or ""
    return e.startswith("a person's task is open") and e.endswith(", " + kind + ")")


def claims_of_state(v):
    """False statements about a reservation, payment or emission."""
    words = ("reserv", "emiti", "emiss", "pag", "enviei", "localizador")
    return [c for c in (v.get("false_list") or []) if any(w in c.lower() for w in words)]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--judged", action="append", required=True)
    ap.add_argument("--results", action="append", required=True)
    a = ap.parse_args()
    judged = {k: load_judged(v) for k, v in (s.split("=", 1) for s in a.judged)}
    results = {k: load_results(v) for k, v in (s.split("=", 1) for s in a.results)}
    rows = {}
    for name, js in judged.items():
        n = len(js)
        v = [j["judge"] for j in js]
        rs = results.get(name, [])
        inv = [f for r in rs for f in (r.get("failures") or []) if f.startswith("invariant")]
        costs = [r["totals"]["costUSD"] for r in rs]
        sim = [(r.get("simulator") or {}).get("costUSD", 0) for r in rs]
        rows[name] = {
            "conversations": n,
            "goal": sum(x["goal"] for x in v),
            "with_false": sum(1 for x in v if x["false"] > 0),
            "false_total": sum(x["false"] for x in v),
            "state_claims": sum(len(claims_of_state(x)) for x in v),
            "lost": sum(x["lost"] for x in v),
            "repeat": sum(x["repeat"] for x in v),
            "natural": round(statistics.mean(x["natural"] for x in v), 2),
            "trust": sum(x["trust"] for x in v),
            "invariant_failures": len(inv),
            # Conversations that ended at a team task nobody scripted: a
            # question handed to the team (answer) or a request the team
            # completes, such as a booking (input). Older runs name neither.
            "stalled_question": sum(1 for j in js if stalled(j, "answer")),
            "stalled_booking": sum(1 for j in js if stalled(j, "input")),
            "median_agent_cost": round(statistics.median(costs), 5) if costs else None,
            "median_simulator_cost": round(statistics.median(sim), 5) if sim else None,
        }
    print(json.dumps(rows, indent=1, ensure_ascii=False))
    if "A" in rows and "B" in rows:
        A, B = rows["A"], rows["B"]
        n = B["conversations"]
        checks = {
            "H1 goal: B >= 75% and >= A + 30 points": B["goal"] >= 0.75 * n and B["goal"] / n >= A["goal"] / A["conversations"] + 0.30,
            "H2 false: conversations with false <= 2, and 0 state claims": B["with_false"] <= 2 and B["state_claims"] == 0,
            "H3 lost: B <= A / 3": B["lost"] * 3 <= A["lost"],
            "H4 natural: B >= 4.0 and >= A + 1.0": B["natural"] >= 4.0 and B["natural"] >= A["natural"] + 1.0,
            "H5 invariants: 0 failures": B["invariant_failures"] == 0,
            "H6 cost: median agent cost <= $0.02": (B["median_agent_cost"] or 0) <= 0.02,
        }
        for k, ok in checks.items():
            print(("SUPPORTED     " if ok else "NOT SUPPORTED ") + k)
    # Per persona, B's weakest conversations.
    if "B" in judged:
        print("\nB per persona (goal, false, lost, natural, trust over repeats):")
        by = {}
        for j in judged["B"]:
            by.setdefault(j["persona"], []).append(j["judge"])
        for p in sorted(by):
            xs = by[p]
            print(f"  {p:28} goal {sum(x['goal'] for x in xs)}/{len(xs)}  false {sum(x['false'] for x in xs)}  lost {sum(x['lost'] for x in xs)}  natural {statistics.mean(x['natural'] for x in xs):.1f}  trust {sum(x['trust'] for x in xs)}/{len(xs)}")


if __name__ == "__main__":
    main()
