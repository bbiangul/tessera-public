"""Builds knowledge-002's agents: learn-001's clinic with the relevance
filter off (rel-off) or on (rel-on), each with a `dev` and a `test`
EvalSuite. R2 is a binary built with the handoff wording.

  python3 bench/relevance/build.py
"""
import json, os, shutil, sys

ROOT = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(ROOT, "../learn"))
from build import AGENT, EMPTY, SUITE  # noqa: E402

if __name__ == "__main__":
    msgs = json.load(open(os.path.join(ROOT, "messages.json")))
    for name, rel in (("rel-off", "false"), ("rel-on", "true")):
        d = os.path.join(ROOT, name)
        shutil.rmtree(d, ignore_errors=True)
        os.makedirs(os.path.join(d, "evals"))
        agent = AGENT.replace("knowledge: { bases: [publico, licoes] }", "knowledge: { bases: [publico, licoes], relevance: " + rel + " }")
        assert agent != AGENT
        open(os.path.join(d, "agent.yaml"), "w").write(agent)
        open(os.path.join(d, "licoes.md"), "w").write(EMPTY)
        for split in ("dev", "test", "confirm"):
            out = [SUITE.format(name=split)]
            for m in msgs[split]:
                out.append(f"    - id: {m['id']}\n      simulator: scripted\n      caller: {{ channel: whatsapp }}\n      turns:\n        - user: {json.dumps(m['message'], ensure_ascii=False)}\n")
            open(os.path.join(d, "evals", split + ".evalsuite.yaml"), "w").write("".join(out))
