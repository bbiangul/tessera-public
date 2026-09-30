"""Builds pdf-001's agents: knowledge-001's clinic (check and relevance
as today) reading the documents as Markdown (md), PDF (pdf) or DOCX
(docx), each with knowledge-001's EvalSuite.

  python3 bench/pdf/build.py
"""
import json, os, shutil, sys

ROOT = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(ROOT, "../knowledge"))
import build as kb  # noqa: E402

PUBLIC = ["convenios", "precos-particular", "pagamento", "cancelamento", "localizacao", "pos-extracao", "clareamento", "avisos"]

if __name__ == "__main__":
    qs = json.load(open(os.path.join(ROOT, "../knowledge/questions.json")))
    for variant, path in (("md", "../../knowledge/docs/{}.md"), ("pdf", "../pdf/{}.pdf"), ("docx", "../docx/{}.docx")):
        d = os.path.join(ROOT, "agent-" + variant)
        shutil.rmtree(d, ignore_errors=True)
        os.makedirs(os.path.join(d, "evals"))
        k = ", knowledge: { bases: [publico, interno] }"
        agent = kb.AGENT.format(staff=k, patient=k)
        files = ", ".join(json.dumps(path.format(n)) for n in PUBLIC)
        agent += kb.KB.replace(kb.KB[kb.KB.index("sources: [ { files: [\"../docs/convenios.md\""):kb.KB.index("] } ]") + 5], "sources: [ { files: [" + files + "] } ]", 1)
        agent = agent.replace('"../docs/interno-descontos.md"', json.dumps(path.format("interno-descontos")))
        open(os.path.join(d, "agent.yaml"), "w").write(agent)
        src = os.path.join(ROOT, "../knowledge/kb-check/evals/knowledge.evalsuite.yaml")
        shutil.copy(src, os.path.join(d, "evals", "knowledge.evalsuite.yaml"))
