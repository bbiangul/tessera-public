"""Builds plugin-001's parity agent: knowledge-001's clinic whose documents
come from a sandboxed plugin: docs/list-public and docs/list-internal
list them, docs/get returns each; 7 as text, 2 as base64 DOCX
(convenios, interno-descontos, from bench/pdf/docx).

  python3 bench/plugin/build.py
"""
import base64, json, os, shutil, sys

ROOT = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(ROOT, "../knowledge"))
import build as kb  # noqa: E402

DOCS = os.path.join(ROOT, "../knowledge/docs")
PUBLIC = ["convenios", "precos-particular", "pagamento", "cancelamento", "localizacao", "pos-extracao", "clareamento", "avisos"]
AS_DOCX = {"convenios", "interno-descontos"}

PLUGIN = """---
apiVersion: plugins.tessera.dev/v1alpha1
kind: PluginManifest
metadata: { name: docs, version: 0.1.0 }
spec:
  actions:
    list-public: { input: {}, output: list, effect: read }
    list-internal: { input: {}, output: list, effect: read }
    get: { input: { id: string }, output: map, effect: read }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: publico, namespace: kb }
spec:
  description: Políticas, preços e FAQ da clínica (de um plugin).
  sources:
    - list: { act: docs/list-public }
      fetch: { act: docs/get, with: { id: "${ inputs.id }" } }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: interno, namespace: kb }
spec:
  description: Política interna de descontos (de um plugin).
  sources:
    - list: { act: docs/list-internal }
      fetch: { act: docs/get, with: { id: "${ inputs.id }" } }
  audiences: [equipe]
"""

SANDBOX = '''# plugin-001: a document library.

def list_public(state):
    return [d for d in state["files"] if not d["internal"]]

def list_internal(state):
    return [d for d in state["files"] if d["internal"]]

def get(state, id):
    return state["content"][id]
'''

if __name__ == "__main__":
    d = os.path.join(ROOT, "agent")
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(os.path.join(d, "evals"))
    k = ", knowledge: { bases: [publico, interno] }"
    open(os.path.join(d, "agent.yaml"), "w").write(kb.AGENT.format(staff=k, patient=k) + PLUGIN)
    open(os.path.join(d, "evals", "docs.sandbox.star"), "w").write(SANDBOX)
    files, content = [], {}
    for n in PUBLIC + ["interno-descontos"]:
        if n in AS_DOCX:
            name = n + ".docx"
            content[n] = {"base64": base64.b64encode(open(os.path.join(ROOT, "../pdf/docx", name), "rb").read()).decode()}
        else:
            name = n + ".md"
            content[n] = {"text": open(os.path.join(DOCS, name)).read()}
        files.append({"id": n, "name": name, "modified": "2026-09-01", "internal": n == "interno-descontos"})
    suite = open(os.path.join(ROOT, "../knowledge/kb-check/evals/knowledge.evalsuite.yaml")).read()
    head, scenarios = suite.split("  scenarios:\n", 1)
    state = json.dumps({"files": files, "content": content}, ensure_ascii=False)
    head += "  sandboxes:\n    - plugin: docs\n      script: docs.sandbox.star\n      state: " + state + "\n"
    open(os.path.join(d, "evals", "knowledge.evalsuite.yaml"), "w").write(head + "  scenarios:\n" + scenarios)
