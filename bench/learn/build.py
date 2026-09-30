"""Builds learn-001's agents (docs/research/learn-001-preregistration.md).

  python3 bench/learn/build.py             # phase1/ and before/
  python3 bench/learn/build.py --after     # after/, with bench/learn/licoes-accepted.md

phase1: the learning scenarios, each with the reception's answer as a
human turn. before/after: the held-out questions, with an empty lessons
file or the accepted one.
"""
import json, os, shutil, sys

ROOT = os.path.dirname(os.path.abspath(__file__))

AGENT = """apiVersion: agents.tessera.dev/v1alpha1
kind: Agent
metadata: { name: clinica, namespace: learn }
spec:
  description: Recepção da Clínica Sorriso Vila.
  messages:
    notUnderstood: "Desculpe, não entendi. Pode falar com a recepção pelo (11) 3000-0000."
  humanInTheLoop: { queue: recepcao }
  audiences:
    - name: equipe
      who: "has(caller.roles) && size(caller.roles) > 0"
      functions: []
      freeMode: { instructions: "Ajude a equipe da recepção.", self: false }
    - name: paciente
      functions: [enviar-exame]
      freeMode:
        instructions: "Tire dúvidas dos pacientes sobre a clínica."
        self: false
        knowledge: { bases: [publico, licoes] }
        functions:
          - function: perguntar-recepcao
            when: "a dúvida não é respondida por FACTS.documents, ou é sobre o próprio caso do paciente (a consulta, o exame, o orçamento ou o pagamento dele): uma pessoa da recepção precisa responder"
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: publico, namespace: learn }
spec:
  description: Políticas, preços e FAQ da clínica.
  sources: [ { files: ["../../knowledge/docs/convenios.md", "../../knowledge/docs/precos-particular.md", "../../knowledge/docs/pagamento.md", "../../knowledge/docs/cancelamento.md", "../../knowledge/docs/localizacao.md", "../../knowledge/docs/pos-extracao.md", "../../knowledge/docs/clareamento.md"] } ]
---
apiVersion: agents.tessera.dev/v1alpha1
kind: KnowledgeBase
metadata: { name: licoes, namespace: learn }
spec:
  description: Lições aprendidas com as respostas da recepção (tessera learn).
  sources: [ { files: ["licoes.md"] } ]
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: perguntar-recepcao, namespace: learn }
spec:
  description: Asks the reception a question the agent cannot answer, and relays the answer.
  steps:
    - id: resposta
      escalate:
        queue: recepcao
        task: answer
        message: "Paciente pergunta: {{ event.message }}"
        whileWaiting: { template: "Vou perguntar para a recepção e já te respondo por aqui." }
  output: { template: "{{ steps.resposta.output }}" }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: Function
metadata: { name: enviar-exame, namespace: learn }
spec:
  description: A patient sends an exam file to the clinic.
  selector: { intent: enviar-exame, description: "O paciente quer mandar um arquivo de exame (raio-x, tomografia) para a clínica." }
  output: { template: "Pode mandar o arquivo por aqui que a recepção anexa ao seu prontuário." }
---
apiVersion: agents.tessera.dev/v1alpha1
kind: HumanQueue
metadata: { name: recepcao, namespace: learn }
spec: { description: A recepção. }   # no roles: the EvalSuite's human turn answers
"""

EMPTY = "# Lições aprendidas\n\nRespostas da equipe aprovadas como lições (tessera learn).\n"

SUITE = """apiVersion: agents.tessera.dev/v1alpha1
kind: EvalSuite
metadata: {{ name: {name}, namespace: learn }}
spec:
  description: learn-001 {name}; transcripts only, judged outside.
  now: "2026-09-27T09:00:00-03:00"
  scenarios:
"""


def write(name, lessons, scenarios, suite):
    d = os.path.join(ROOT, name)
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(os.path.join(d, "evals"))
    open(os.path.join(d, "agent.yaml"), "w").write(AGENT)
    open(os.path.join(d, "licoes.md"), "w").write(lessons)
    out = [SUITE.format(name=suite)]
    for s in scenarios:
        out.append(f"    - id: {s['id']}\n      simulator: scripted\n      caller: {{ channel: whatsapp }}\n      turns:\n        - user: {json.dumps(s['message'], ensure_ascii=False)}\n")
        if "human" in s:
            out.append(f"        - human: {json.dumps(s['human'], ensure_ascii=False)}\n")
    open(os.path.join(d, "evals", suite + ".evalsuite.yaml"), "w").write("".join(out))


if __name__ == "__main__":
    if "--set2" in sys.argv:  # learn-002
        after2 = json.load(open(os.path.join(ROOT, "after2.json")))["questions"]
        if "--after" in sys.argv:
            write("after2", open(os.path.join(ROOT, "licoes-accepted2.md")).read(), after2, "after")
        else:
            write("phase1b", EMPTY, json.load(open(os.path.join(ROOT, "learning2.json")))["scenarios"], "learning")
            write("before2", EMPTY, after2, "after")
        sys.exit(0)
    after = json.load(open(os.path.join(ROOT, "after.json")))["questions"]
    if "--after" in sys.argv:
        write("after", open(os.path.join(ROOT, "licoes-accepted.md")).read(), after, "after")
    else:
        write("phase1", EMPTY, json.load(open(os.path.join(ROOT, "learning.json")))["scenarios"], "learning")
        write("before", EMPTY, after, "after")
