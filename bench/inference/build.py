"""Builds knowledge-003's agent: knowledge-001's clinic as today, with
`dev` and `test` EvalSuites of the inference messages.

  python3 bench/inference/build.py
"""
import json, os, shutil

ROOT = os.path.dirname(os.path.abspath(__file__))

if __name__ == "__main__":
    msgs = json.load(open(os.path.join(ROOT, "messages.json")))
    d = os.path.join(ROOT, "agent")
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(os.path.join(d, "evals"))
    agent = open(os.path.join(ROOT, "../knowledge/kb-check/agent.yaml")).read().replace('"../docs/', '"../../knowledge/docs/')
    open(os.path.join(d, "agent.yaml"), "w").write(agent)
    for split in ("dev", "test"):
        out = ["""apiVersion: agents.tessera.dev/v1alpha1
kind: EvalSuite
metadata: { name: %s, namespace: kb }
spec:
  description: knowledge-003; transcripts only, judged outside.
  now: "2026-09-28T09:00:00-03:00"
  scenarios:
""" % split]
        for m in msgs[split]:
            out.append(f"    - id: {m['id']}\n      simulator: scripted\n      caller: {{ channel: whatsapp }}\n      turns:\n        - user: {json.dumps(m['message'], ensure_ascii=False)}\n")
        open(os.path.join(d, "evals", split + ".evalsuite.yaml"), "w").write("".join(out))
