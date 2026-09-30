"""Builds dispute-001's agent: learn-001's clinic with bench/dispute/lessons.md,
and `dev` and `test` EvalSuites of two-turn conversations.

  python3 bench/dispute/build.py
"""
import json, os, shutil, sys

ROOT = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(ROOT, "../learn"))
from build import AGENT, SUITE  # noqa: E402

if __name__ == "__main__":
    conv = json.load(open(os.path.join(ROOT, "conversations.json")))
    d = os.path.join(ROOT, "agent")
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(os.path.join(d, "evals"))
    open(os.path.join(d, "agent.yaml"), "w").write(AGENT)
    shutil.copy(os.path.join(ROOT, "lessons.md"), os.path.join(d, "licoes.md"))
    for split in ("dev", "test"):
        out = [SUITE.format(name=split)]
        for c in conv[split]:
            out.append(f"    - id: {c['id']}\n      simulator: scripted\n      caller: {{ channel: whatsapp }}\n      turns:\n")
            for t in c["turns"]:
                out.append(f"        - user: {json.dumps(t, ensure_ascii=False)}\n")
        open(os.path.join(d, "evals", split + ".evalsuite.yaml"), "w").write("".join(out))
