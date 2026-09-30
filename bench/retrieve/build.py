"""Builds retrieve-001's agents: knowledge-001's clinic with a Function
orcamento that answers price questions, with a retrieve step (with) or
without one (without).

  python3 bench/retrieve/build.py
"""
import json, os, shutil, sys

ROOT = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(ROOT, "../knowledge"))
import build as kb  # noqa: E402

INSTRUCTIONS = ("Answer the patient's price question (MESSAGE) in Portuguese, briefly and kindly, only from PASSAGES (the clinic's own documents). "
                "If PASSAGES do not give the price asked, say you do not have that price here and that the reception gives it at the evaluation. "
                "Never give a price that is not in PASSAGES.")

FUNCTION = """---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: {{ name: orcamento, namespace: kb }}
spec:
  description: Answers a patient's price question from the clinic's price list.
  selector: {{ intent: orcamento, description: "O paciente pergunta o preço ou o valor de um tratamento ou consulta." }}
  steps:
{retrieve}    - id: resposta
      generate:
        instructions: {instructions}
        state: {{ message: "${{ event.message }}", passages: "${{ {passages} }}" }}
  output: {{ template: "{{{{ steps.resposta.output }}}}" }}
"""

if __name__ == "__main__":
    qs = json.load(open(os.path.join(ROOT, "messages.json")))["questions"]
    for variant in ("with", "without"):
        d = os.path.join(ROOT, variant)
        shutil.rmtree(d, ignore_errors=True)
        os.makedirs(os.path.join(d, "evals"))
        agent = kb.AGENT.format(staff="", patient=", knowledge: { bases: [publico, interno] }").replace("functions: [enviar-exame]", "functions: [enviar-exame, orcamento]")
        agent += kb.KB.replace("../docs/", "../../knowledge/docs/")
        if variant == "with":
            fn = FUNCTION.format(retrieve='    - { id: docs, retrieve: { bases: [publico], query: "event.message" } }\n', instructions=json.dumps(INSTRUCTIONS), passages="steps.docs.output")
        else:
            fn = FUNCTION.format(retrieve="", instructions=json.dumps(INSTRUCTIONS), passages="[]")
        open(os.path.join(d, "agent.yaml"), "w").write(agent + fn)
        out = ["""apiVersion: agents.tessera.dev/v1alpha1
kind: EvalSuite
metadata: { name: orcamento, namespace: kb }
spec:
  description: retrieve-001; transcripts only, judged outside.
  now: "2026-09-27T09:00:00-03:00"
  scenarios:
"""]
        for q in qs:
            out.append(f"    - id: {q['id']}\n      simulator: scripted\n      caller: {{ channel: whatsapp }}\n      turns:\n        - user: {json.dumps(q['message'], ensure_ascii=False)}\n")
        open(os.path.join(d, "evals", "orcamento.evalsuite.yaml"), "w").write("".join(out))
