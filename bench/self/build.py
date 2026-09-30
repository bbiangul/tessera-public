"""Builds the self-001 variants (docs/research/self-001-preregistration.md).

  python3 bench/self/build.py

Part A copies bench/traveler-freemode/freemode into bench/self/traveler/<variant>;
Part B copies bench/backoffice/agent-008 into bench/self/clinic/<variant>.
Each variant gets one EvalSuite, `self`, with the set's messages; the
traveler's own suites are left out (the stale variants drop search-hotels,
which they test). Re-running rebuilds the variants from the sources.
"""
import json, os, re, shutil

ROOT = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(ROOT, "../.."))
TRAVELER = os.path.join(REPO, "bench/traveler-freemode/freemode")
CLINIC = os.path.join(REPO, "bench/backoffice/agent-008/agent.yaml")

LISTED = ("Você é o assistente de viagens no chat. Pelo chat você busca passagens aéreas e hospedagem e anota as preferências de viagem da pessoa; "
          "não faz reservas, pacotes, aluguel de carro, seguro nem vistos. ")
RULES = ("Quando a dúvida depende de dados atuais (preços, disponibilidade, regras de uma companhia ou de um país), diga que não tem esse dado aqui e "
         "ofereça o que você faz: buscar passagens ou hospedagem com as datas e o destino da pessoa.")
# bare keeps the rule about current data, without naming what it does.
BARE = ("Você é o assistente de viagens no chat. Quando a dúvida depende de dados atuais (preços, disponibilidade, regras de uma companhia ou de um país), "
        "diga que não tem esse dado aqui.")

TRAVELER_VARIANTS = {
    "listed": dict(instructions=LISTED + RULES, self=False, hotels=True),
    "bare": dict(instructions=BARE, self=False, hotels=True),
    "self": dict(instructions=BARE, self=True, hotels=True),
    "stale-listed": dict(instructions=LISTED + RULES, self=False, hotels=False),
    "stale-self": dict(instructions=BARE, self=True, hotels=False),
}


def yaml_str(s):
    return json.dumps(s, ensure_ascii=False)


def build_traveler():
    msgs = json.load(open(os.path.join(ROOT, "traveler-messages.json")))
    for name, v in TRAVELER_VARIANTS.items():
        dst = os.path.join(ROOT, "traveler", name)
        shutil.rmtree(dst, ignore_errors=True)
        shutil.copytree(TRAVELER, dst, ignore=shutil.ignore_patterns("README.md"))
        agent = open(os.path.join(dst, "agent.yaml")).read()
        free = "  freeMode:\n    instructions: " + yaml_str(v["instructions"]) + "\n"
        free += "    self: true\n" if v["self"] else "    self: false\n"  # default on since #60
        agent, n = re.subn(r"  freeMode:\n    instructions: .*\n", free, agent)
        assert n == 1, name
        agent = agent.replace("  description: Assistente de viagens (passagens, hospedagem, preferências).",
                              "  description: " + ("Assistente de viagens (passagens, hospedagem, preferências)." if v["hotels"] else "Assistente de viagens (passagens, preferências)."))
        if not v["hotels"]:
            # notUnderstood names hotels too; the drift is the free mode's.
            os.remove(os.path.join(dst, "functions/search-hotels.yaml"))
        open(os.path.join(dst, "agent.yaml"), "w").write(agent)
        evals = os.path.join(dst, "evals")
        for f in os.listdir(evals):
            if f.endswith(".evalsuite.yaml"):
                os.remove(os.path.join(evals, f))
        suite = open(os.path.join(TRAVELER, "evals/offscript.evalsuite.yaml")).read()
        head = suite[:suite.index("  scenarios:")]
        head = head.replace("name: offscript", "name: self").replace(
            "Off-script messages (traveler-freemode-001); transcripts only, judged outside.",
            "Messages about the agent itself (self-001); transcripts only, judged outside.")
        out = [head, "  scenarios:\n"]
        for m in msgs:
            out.append(f"    - id: {m['id']}\n      simulator: scripted\n      turns:\n        - user: {yaml_str(m['message'])}\n")
        open(os.path.join(evals, "self.evalsuite.yaml"), "w").write("".join(out))


CLINIC_SANDBOX = '''# Sandbox of the clinic plugin for self-001: a fixed schedule and info.

def info(state):
    return state["info"]

def schedule(state, dentistId):
    return state["schedules"].get(dentistId, [])

def discount(state, patientId, percent):
    state["discounts"].append({"patientId": patientId, "percent": percent})
    return {"ok": True}

def block(state, dentistId, day):
    state["blocks"].append({"dentistId": dentistId, "day": day})
    return {"ok": True}

def discounts_report(state):
    return ["%s: %d%%" % (d["patientId"], d["percent"]) for d in state["discounts"]]
'''

CALLERS = {
    "patient": "{ channel: whatsapp, role: patient, patientId: p1 }",
    "dentist": "{ channel: whatsapp, member: bruno, roles: [dentista], attributes: { dentistId: D1 } }",
    "owner": "{ channel: whatsapp, member: dora, roles: [dono] }",
}


def build_clinic():
    msgs = json.load(open(os.path.join(ROOT, "clinic-messages.json")))
    src = open(CLINIC).read()
    for name, self_ in (("bare", False), ("self", True)):
        dst = os.path.join(ROOT, "clinic", name)
        shutil.rmtree(dst, ignore_errors=True)
        os.makedirs(os.path.join(dst, "evals"))
        agent = src
        if True:  # default on since #60: bare says false
            flag = "true" if self_ else "false"
            agent, n = re.subn(r"(      freeMode: \{ instructions: \"[^\"]*\", tasks: true)( \})", r"\1, self: " + flag + r"\2", agent)
            assert n == 2, n  # dono, equipe
            agent, n = re.subn(r"(      freeMode:\n        instructions: \"[^\"]*\"\n)", r"\1        self: " + flag + r"\n", agent)
            assert n == 2, n  # dentista, paciente
        open(os.path.join(dst, "agent.yaml"), "w").write(agent)
        open(os.path.join(dst, "evals/clinic.sandbox.star"), "w").write(CLINIC_SANDBOX)
        out = ["""apiVersion: agents.tessera.dev/v1alpha1
kind: EvalSuite
metadata: { name: self, namespace: clinica-bo }
spec:
  description: Messages about the agent itself, per audience (self-001); transcripts only, judged outside.
  now: "2026-09-23T09:00:00-03:00"
  sandboxes:
    - plugin: clinic
      script: clinic.sandbox.star
      state:
        info: { endereco: "Rua das Flores, 100", horario: "seg a sex, 8h às 18h", telefone: "(11) 3000-0000" }
        schedules: { D1: ["seg 9h Ana", "seg 14h Caio", "sex 10h Bia"] }
        discounts: []
        blocks: []
  scenarios:
"""]
        for m in msgs:
            out.append(f"    - id: {m['id']}\n      simulator: scripted\n      caller: {CALLERS[m['caller']]}\n      turns:\n        - user: {yaml_str(m['message'])}\n")
        open(os.path.join(dst, "evals/self.evalsuite.yaml"), "w").write("".join(out))


def build_guard():
    """guard-001..003 G4: the traveler `listed` with the guard's messages."""
    msgs = json.load(open(os.path.join(ROOT, "guard-messages.json")))
    src = os.path.join(ROOT, "traveler", "listed")
    dst = os.path.join(ROOT, "guard", "listed")
    shutil.rmtree(dst, ignore_errors=True)
    shutil.copytree(src, dst)
    suite = os.path.join(dst, "evals/self.evalsuite.yaml")
    s = open(suite).read()
    s = s[:s.index("  scenarios:")].replace("name: self", "name: guard").replace("(self-001)", "(guard-001 G4)")
    s += "  scenarios:\n" + "".join(f"    - id: {m['id']}\n      simulator: scripted\n      turns:\n        - user: {yaml_str(m['message'])}\n" for m in msgs)
    os.remove(suite)
    open(os.path.join(dst, "evals/guard.evalsuite.yaml"), "w").write(s)


if __name__ == "__main__":
    build_traveler()
    build_clinic()
    build_guard()
