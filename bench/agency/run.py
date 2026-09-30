"""agency-001 harness: plays the personas against one variant of the
traveler, with the llm simulator and the world's sandboxes.

  python3 bench/agency/run.py --variant bench/agency/baseline --split dev \
      --repeats 1 --out bench/results/agency-001/A-dev [--only d01,d02] [--parallel 6]

For each repeat and persona it builds the variant with a generated
EvalSuite (evals/agency.evalsuite.yaml) and the world's sandboxes for the
plugins the variant has, then runs `tessera test --only agency/<id>`, one
process per persona. Results: <out>/r<N>/<persona>.json. Keys come from
the environment (TYPESAFE_API_KEY, OPENROUTER_API_KEY); nothing is printed.
"""
import argparse, json, os, re, shutil, subprocess, sys, tempfile
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "../.."))
WORLD = os.path.join(HERE, "world")
GENERATE = "openrouter:deepseek/deepseek-v4.1-flash"
DECIDE = "typesafe:jev-1.13.0"

# Sandboxes the world has, by plugin name: script and initial state.
SANDBOXES = {
    "booking": ("booking.sandbox.star", {}),
    "routes": ("routes.sandbox.star", {"minutes": {}}),
    "agency": ("agency.sandbox.star", "agency.state.json"),
    "web": ("web.sandbox.star", {}),
}


def plugins_of(variant):
    names = set()
    for dirpath, _, files in os.walk(variant):
        for f in files:
            if f.endswith(".yaml"):
                text = open(os.path.join(dirpath, f)).read()
                if "kind: PluginManifest" in text:
                    m = re.search(r"^metadata:\s*\n\s+name:\s*(\S+)", text, re.M) or re.search(r"metadata:\s*\{\s*name:\s*([\w-]+)", text)
                    if m:
                        names.add(m.group(1))
    return names


def suite(personas, now, namespace, plugins):
    boxes = []
    for name, (script, state) in SANDBOXES.items():
        if name not in plugins:
            continue
        if isinstance(state, str):
            state = json.load(open(os.path.join(WORLD, state)))
        boxes.append({"plugin": name, "script": script, "state": state})
    scenarios = []
    for p in personas:
        sc = {"id": p["id"], "simulator": "llm", "maxTurns": 14, "caller": p.get("caller") or {},
              "persona": {"goal": p["goal"], "style": p.get("style", ""), "knows": p.get("knows") or {}}}
        if p.get("humans"):
            sc["humans"] = p["humans"]
        scenarios.append(sc)
    return {"apiVersion": "agents.tessera.dev/v1alpha1", "kind": "EvalSuite",
            "metadata": {"name": "agency", "namespace": namespace},
            "spec": spec_team({"description": "agency-001 personas", "now": now, "sandboxes": boxes, "scenarios": scenarios})}


def spec_team(spec):
    """The simulated team answers what no persona scripted (team-001), when
    the world has its facts."""
    if os.path.exists(os.path.join(WORLD, "team-facts.md")) and os.environ.get("AGENCY_NO_TEAM") != "1":
        spec["team"] = {"facts": "team-facts.md"}
    return spec


def namespace_of(variant):
    text = open(os.path.join(variant, "agent.yaml")).read()
    return re.search(r"namespace:\s*(\S+)", text).group(1)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--variant", required=True)
    ap.add_argument("--split", required=True, choices=["dev", "test"])
    ap.add_argument("--repeats", type=int, default=1)
    ap.add_argument("--first-repeat", type=int, default=1)
    ap.add_argument("--out", required=True)
    ap.add_argument("--only", default="")
    ap.add_argument("--parallel", type=int, default=6)
    ap.add_argument("--personas", default=os.path.join(HERE, "personas.json"))
    ap.add_argument("--binary", default="", help="a tessera binary to use instead of building the working tree")
    ap.add_argument("--max-cost", type=float, default=0.4, help="per persona run, agent and simulator")
    a = ap.parse_args()
    for k in ("TYPESAFE_API_KEY", "OPENROUTER_API_KEY"):
        if not os.environ.get(k):
            sys.exit(f"set {k}")
    data = json.load(open(a.personas))
    personas = data[a.split]
    if a.only:
        keep = set(a.only.split(","))
        personas = [p for p in personas if p["id"] in keep or p["id"].split("-")[0] in keep]
    tmp = tempfile.mkdtemp(prefix="agency-")
    binary = os.path.abspath(a.binary) if a.binary else os.path.join(tmp, "tessera")
    if not a.binary:
        subprocess.run(["go", "build", "-o", binary, "./cmd/tessera"], cwd=ROOT, check=True)
    variant = os.path.abspath(a.variant)
    defs = os.path.join(tmp, "defs")
    shutil.copytree(variant, defs, ignore=shutil.ignore_patterns("evals"))
    os.makedirs(os.path.join(defs, "evals"))
    for f in os.listdir(WORLD):
        if f.endswith(".star") or f == "team-facts.md":
            shutil.copy(os.path.join(WORLD, f), os.path.join(defs, "evals", f))
    doc = suite(personas, data["now"], namespace_of(variant), plugins_of(variant))
    with open(os.path.join(defs, "evals", "agency.evalsuite.yaml"), "w") as f:
        json.dump(doc, f, ensure_ascii=False, indent=1)  # JSON is YAML

    jobs = [(r, p["id"]) for r in range(a.first_repeat, a.first_repeat + a.repeats) for p in personas]

    def one(job):
        rep, pid = job
        out = os.path.join(os.path.abspath(a.out), f"r{rep}", pid + ".json")
        if os.path.exists(out):
            return f"{pid} r{rep}: kept"
        os.makedirs(os.path.dirname(out), exist_ok=True)
        cmd = [binary, "test", defs, "--decide", DECIDE, "--generate", GENERATE, "--only", "agency/" + pid,
               "--max-cost-usd", str(a.max_cost), "--max-scenario-cost-usd", str(a.max_cost), "--out", out]
        p = subprocess.run(cmd, capture_output=True, text=True)
        if not os.path.exists(out):
            return f"{pid} r{rep}: FAILED {p.stderr.strip()[-300:]}"
        return f"{pid} r{rep}: done"

    with ThreadPoolExecutor(a.parallel) as ex:
        for line in ex.map(one, jobs):
            print(line, flush=True)
    shutil.rmtree(tmp)


if __name__ == "__main__":
    main()
