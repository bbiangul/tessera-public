"""Builds lifecycle-001's agents (docs/research/lifecycle-001-preregistration.md).

  python3 bench/lifecycle/build.py                 # lc/: expiry, usage and contradiction suites
  python3 bench/lifecycle/build.py --after FILE    # lc-after/: the after suite, with FILE as lessons
"""
import json, os, shutil, sys

ROOT = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(ROOT, "../learn"))
from build import AGENT, SUITE  # noqa: E402


def agent(name, lessons):
    d = os.path.join(ROOT, name)
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(os.path.join(d, "evals"))
    open(os.path.join(d, "agent.yaml"), "w").write(AGENT)
    shutil.copy(lessons, os.path.join(d, "licoes.md"))
    return d


def suite(d, name, scenarios):
    out = [SUITE.format(name=name)]
    for s in scenarios:
        turns = s.get("turns") or [s["message"]]
        out.append(f"    - id: {s['id']}\n      simulator: scripted\n      caller: {{ channel: whatsapp }}\n      turns:\n")
        for t in turns:
            out.append(f"        - user: {json.dumps(t, ensure_ascii=False)}\n")
        if "human" in s:
            out.append(f"        - human: {json.dumps(s['human'], ensure_ascii=False)}\n")
    open(os.path.join(d, "evals", name + ".evalsuite.yaml"), "w").write("".join(out))


if __name__ == "__main__":
    sc = json.load(open(os.path.join(ROOT, "scenarios.json")))
    if "--after" in sys.argv:
        d = agent("lc-after", sys.argv[sys.argv.index("--after") + 1])
        suite(d, "after", sc["after"])
    else:
        d = agent("lc", os.path.join(ROOT, "lessons-start.md"))
        suite(d, "expiry", sc["expiry"])
        suite(d, "usage", sc["usage"])
        suite(d, "contradiction", sc["contradiction"])
