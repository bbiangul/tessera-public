"""Builds knowledge-001's variants (docs/research/knowledge-001-preregistration.md).

  python3 bench/knowledge/build.py
writes bench/knowledge/{none,kb,kb-check}/ (agent + EvalSuite `knowledge`).
"""
import json, os, shutil

ROOT = os.path.dirname(os.path.abspath(__file__))

AGENT = """apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: {{ name: clinica, namespace: kb }}
spec:
  description: Recepção da Clínica Sorriso Vila.
  messages:
    notUnderstood: "Desculpe, não entendi. Pode falar com a recepção pelo (11) 3000-0000."
  audiences:
    - name: equipe
      who: "has(caller.roles) && size(caller.roles) > 0"
      functions: []
      freeMode: {{ instructions: "Ajude a equipe da recepção com dúvidas de processo.", self: false{staff} }}
    - name: paciente
      functions: [enviar-exame]
      freeMode: {{ instructions: "Tire dúvidas dos pacientes sobre a clínica. Para o que você não souber, indique a recepção.", self: false{patient} }}
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: {{ name: enviar-exame, namespace: kb }}
spec:
  description: A patient sends an exam file to the clinic.
  selector: {{ intent: enviar-exame, description: "O paciente quer mandar um arquivo de exame (raio-x, tomografia) para a clínica." }}
  output: {{ template: "Pode mandar o arquivo por aqui que a recepção anexa ao seu prontuário." }}
"""

KB = """---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: publico, namespace: kb }
spec:
  description: Políticas, preços e FAQ da clínica.
  sources: [ { files: ["../docs/convenios.md", "../docs/precos-particular.md", "../docs/pagamento.md", "../docs/cancelamento.md", "../docs/localizacao.md", "../docs/pos-extracao.md", "../docs/clareamento.md", "../docs/avisos.md"] } ]
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: interno, namespace: kb }
spec:
  description: Política interna de descontos.
  sources: [ { files: ["../docs/interno-descontos.md"] } ]
  audiences: [equipe]
"""

CALLERS = {"patient": "{ channel: whatsapp }", "staff": "{ channel: whatsapp, member: rita, roles: [recepcao] }"}

VARIANTS = {"none": None, "kb": "false", "kb-check": "true", "kb-relevance": "true, relevance: true"}  # kb-relevance: knowledge-002 Q4

if __name__ == "__main__":
    qs = json.load(open(os.path.join(ROOT, "questions.json")))
    for name, check in VARIANTS.items():
        d = os.path.join(ROOT, name)
        shutil.rmtree(d, ignore_errors=True)
        os.makedirs(os.path.join(d, "evals"))
        k = "" if check is None else f", knowledge: {{ bases: [publico, interno], check: {check} }}"
        agent = AGENT.format(staff=k, patient=k)
        if check is not None:
            agent += KB
        open(os.path.join(d, "agent.yaml"), "w").write(agent)
        out = ["""apiVersion: agents.tessera.dev/v1alpha1
kind: EvalSuite
metadata: { name: knowledge, namespace: kb }
spec:
  description: knowledge-001; transcripts only, judged outside.
  now: "2026-09-23T09:00:00-03:00"
  scenarios:
"""]
        for q in qs:
            out.append(f"    - id: {q['id']}\n      simulator: scripted\n      caller: {CALLERS[q['caller']]}\n      turns:\n        - user: {json.dumps(q['message'], ensure_ascii=False)}\n")
        open(os.path.join(d, "evals/knowledge.evalsuite.yaml"), "w").write("".join(out))
