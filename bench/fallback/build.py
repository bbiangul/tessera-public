"""Builds fallback-001's agent (knowledge-001's clinic) with one EvalSuite
of the controls and traps.

  python3 bench/fallback/build.py
"""
import json, os, shutil

ROOT = os.path.dirname(os.path.abspath(__file__))

if __name__ == "__main__":
    msgs = json.load(open(os.path.join(ROOT, "messages.json")))
    d = os.path.join(ROOT, "agent")
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(os.path.join(d, "evals"))
    open(os.path.join(d, "agent.yaml"), "w").write(open(os.path.join(ROOT, "../knowledge/kb-check/agent.yaml")).read().replace('"../docs/', '"../../knowledge/docs/'))
    out = ["""apiVersion: agents.tessera.dev/v1alpha1
kind: EvalSuite
metadata: { name: fallback, namespace: kb }
spec:
  description: fallback-001; transcripts only, judged outside.
  now: "2026-09-28T09:00:00-03:00"
  scenarios:
"""]
    for m in msgs["controls"] + msgs["traps"]:
        out.append(f"    - id: {m['id']}\n      simulator: scripted\n      caller: {{ channel: whatsapp }}\n      turns:\n        - user: {json.dumps(m['message'], ensure_ascii=False)}\n")
    open(os.path.join(d, "evals", "fallback.evalsuite.yaml"), "w").write("".join(out))
