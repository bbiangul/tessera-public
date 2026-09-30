"""Scores drift-009: runs `tessera drift --json` on each condition's
journals and checks the pre-registered expected signal.

  python3 bench/market/drift_report_score.py bench/results/drift-009
"""
import json, subprocess, sys, os

DEFS = "bench/market/tessera-improvise"
NS = "clinica-bench"


def report(folder):
    out = subprocess.run(["./bin/tessera", "drift", DEFS, "--db", os.path.join(folder, "db", "*.db"), "--namespace", NS, "--since", "8760h", "--json"],
                         capture_output=True, text=True, check=True).stdout
    return json.loads(out)


def action(r, name):
    return next((a for a in r.get("actions") or [] if a["action"] == name), {})


def refusal(a, *words):
    return any(all(w in s["key"] for w in words) for s in a.get("refusals") or [])


def repair(a, field, agree=False):
    return any(field in rp["fields"] and rp["succeeded"] > 0 and (not agree or rp.get("agree")) for rp in a.get("repairs") or [])


def contract_rename(r):
    a = action(r, "clinic/list-appointments")
    c = any("missing label, start" in s["key"] for s in a.get("contracts") or [])  # both the unrepaired CONTRACT and "(repaired by renaming)"
    rn = [s["key"] for s in a.get("renames") or []]
    ok_rename = any(k == "label ← description, start ← startsAt" for k in rn)
    decoy = any("created" in k for k in rn)
    proposal = any(p["kind"] == "binding-output" for p in a.get("proposals") or [])
    return c, ok_rename, decoy, proposal


def main():
    base = sys.argv[1]
    res = {}
    for d in ["D0", "D1", "D2", "D3", "D4", "D5", "D6"]:
        r = report(os.path.join(base, d))
        res[d] = r
        mv = action(r, "clinic/reschedule")
        if d == "D0":
            acts = r.get("actions") or []
            contracts = sum(len(a.get("contracts") or []) for a in acts)
            renames = sum(len(a.get("renames") or []) for a in acts)
            repairs = sum(len(a.get("repairs") or []) for a in acts)
            refusals = [s["key"] for a in acts for s in a.get("refusals") or []]
            alarms = [k for k in refusals if "slot not available" not in k]
            print(f"D0: contracts {contracts} renames {renames} repairs {repairs} refusals {refusals} -> false alarms {contracts + renames + repairs + len(alarms)}")
            continue
        if d == "D1":
            ok = refusal(mv, "reason") and repair(mv, "reason=(asked)")
        elif d == "D2":
            ok = refusal(mv, "R$ 50") and repair(mv, "acceptFee", agree=True)
        elif d == "D3":
            ok = refusal(mv, "3030-4040")
        elif d == "D5":
            ok = refusal(mv, "R$ 500") and not any(rp["succeeded"] for rp in mv.get("repairs") or [])
        else:
            c, rn, decoy, prop = contract_rename(r)
            ok = c and rn and prop and (d == "D4" or not decoy)
        print(f"{d}: expected signal {'FOUND' if ok else 'MISSING'}")
    json.dump(res, open(os.path.join(base, "reports.json"), "w"), ensure_ascii=False, indent=1)


if __name__ == "__main__":
    main()
