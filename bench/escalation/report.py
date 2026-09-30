"""Tables of the escalation benchmark (docs/research/escalation-000-preregistration.md).

  python3 report.py <runs.jsonl> [<judgments.jsonl>]
"""
import collections, json, math, statistics, sys


def wilson(k, n, z=1.96):
    if n == 0:
        return "–"
    p = k / n
    d = 1 + z * z / n
    c = p + z * z / (2 * n)
    m = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n))
    return f"{max(0, (c - m) / d) * 100:.0f}–{min(1, (c + m) / d) * 100:.0f}"


def pct(k, n):
    return f"{k / n * 100:.0f}% ({wilson(k, n)})" if n else "–"


def main():
    runs = [json.loads(l) for l in open(sys.argv[1])]
    judged = [json.loads(l) for l in open(sys.argv[2])] if len(sys.argv) > 2 else []
    by = collections.defaultdict(list)
    for r in runs:
        by[r["competitor"]].append(r)
    print("| Stack | Runs | Errors | Unneeded (answer + partial) | answer | partial | Coach recall | Flaky questions | Mean per-question SD | Cost/run (m$) | p50 latency |")
    print("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
    for c, rs in sorted(by.items()):
        ok = [r for r in rs if not r["error"]]
        un = [r for r in ok if r["label"] in ("answer", "partial")]
        ans = [r for r in ok if r["label"] == "answer"]
        par = [r for r in ok if r["label"] == "partial"]
        coach = [r for r in ok if r["label"] == "coach"]
        perq = collections.defaultdict(list)
        for r in ok:
            perq[r["question"]].append(1 if r["escalated"] else 0)
        flaky = sorted(q for q, v in perq.items() if 0 < sum(v) < len(v))
        sd = statistics.mean(statistics.pstdev(v) for v in perq.values()) if perq else 0
        lat = sorted(r["latencyMs"] for r in ok)
        print(f"| {c} | {len(rs)} | {len(rs) - len(ok)} | {pct(sum(r['escalated'] for r in un), len(un))} | "
              f"{pct(sum(r['escalated'] for r in ans), len(ans))} | {pct(sum(r['escalated'] for r in par), len(par))} | "
              f"{pct(sum(r['escalated'] for r in coach), len(coach))} | {len(flaky)} ({', '.join(flaky)}) | {sd:.2f} | "
              f"{statistics.mean(r['costUSD'] for r in ok) * 1000:.3f} | {lat[len(lat) // 2] / 1000:.2f} s |")
    print()
    print("Escalations per question (of the repeats):")
    qids = sorted({r["question"] for r in runs})
    print("| Question | Label | " + " | ".join(sorted(by)) + " |")
    print("| --- | --- | " + " | ".join("---" for _ in by) + " |")
    for q in qids:
        label = next(r["label"] for r in runs if r["question"] == q)
        cells = []
        for c in sorted(by):
            v = [r["escalated"] for r in by[c] if r["question"] == q and not r["error"]]
            cells.append(f"{sum(v)}/{len(v)}")
        print(f"| {q} | {label} | " + " | ".join(cells) + " |")
    strata = {}
    try:
        import os
        qd = json.load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "questions.json")))
        strata = {q["id"]: q.get("stratum") for q in qd["questions"] if q.get("stratum")}
    except OSError:
        pass
    if any(r["question"] in strata for r in runs):
        print()
        print("| Stack | Unneeded on the near-coach-clause stratum |")
        print("| --- | --- |")
        for c, rs in sorted(by.items()):
            v = [r["escalated"] for r in rs if not r["error"] and strata.get(r["question"]) == "near-coach-clause"]
            print(f"| {c} | {pct(sum(v), len(v))} |")
    if judged:
        print()
        print("| Stack | Judged | Correct | False claims |")
        print("| --- | --- | --- | --- |")
        jb = collections.defaultdict(list)
        for j in judged:
            jb[j["competitor"]].append(j)
        for c, js in sorted(jb.items()):
            print(f"| {c} | {len(js)} | {pct(sum(j['judge']['correct'] for j in js), len(js))} | "
                  f"{sum(j['judge']['false_claim'] for j in js)} |")


if __name__ == "__main__":
    main()
