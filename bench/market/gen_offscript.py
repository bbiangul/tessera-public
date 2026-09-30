"""Generates bench/market/tasks-offscript.json: 20 off-script tasks
(docs/research/rfc0004-baseline-preregistration.md, section 2). Seeded;
no personal data. Reuses gen_tasks' clinic calendar."""
import json, random, sys
# freemode-001's held-out set is seed 32 (python3 gen_offscript.py 32);
# the baseline and development set is seed 31.
SEED = int(sys.argv[1]) if len(sys.argv) > 1 else 31
OUT = "bench/market/tasks-offscript.json" if SEED == 31 else f"bench/market/tasks-offscript-{SEED}.json"
sys.argv = [sys.argv[0], str(SEED), "/dev/null"]
import importlib.util
spec = importlib.util.spec_from_file_location("g", "bench/market/gen_tasks.py")
g = importlib.util.module_from_spec(spec)
spec.loader.exec_module(g)  # builds g.tasks with seed 31 (appointments labelled)

rng = random.Random(SEED * 101)
FACTS = {"endereco": "Rua das Flores, 120, Pinheiros, São Paulo", "estacionamento": "sim, conveniado no prédio ao lado, 2 horas grátis",
         "horario": "segunda a sexta, das 8h às 18h; sábado, das 8h às 12h", "precos": {"limpeza": "R$ 180", "avaliacao": "gratuita", "clareamento": "R$ 900"},
         "convenios": ["Amil Dental", "Odontoprev", "SulAmérica Odonto"], "telefone": "(11) 3030-4040"}
SIDE = [("tem estacionamento?", "estacionamento"), ("qual o endereço de vocês?", "endereco"), ("vocês aceitam Odontoprev?", "convenios"),
        ("quanto custa uma limpeza?", "precos"), ("vocês abrem sábado?", "horario"), ("qual o telefone da clínica?", "telefone")]
INFO = [("quanto custa um clareamento e se aceitam Amil Dental", ["precos", "convenios"]), ("o endereço e se tem estacionamento", ["endereco", "estacionamento"]),
        ("o horário de funcionamento de sábado", ["horario"]), ("se aceitam SulAmérica Odonto e quanto custa a avaliação", ["convenios", "precos"])]
if SEED != 31:  # held-out sets ask their facts in another order
    rng.shuffle(SIDE)
    rng.shuffle(INFO)
kinds = ["side-question"] * 6 + ["info-only"] * 4 + ["uncovered-action"] * 4 + ["small-talk"] * 2 + ["new-request-midway"] * 2 + ["over-reach"] * 2
out = []
for i, (t, k) in enumerate(zip(g.tasks, kinds)):
    name, pid, appt = t["patientName"], t["patientId"], t["appointments"][0]
    target = rng.choice(t["slots"])
    lbl = target["label"]
    t = dict(t, kind=k)
    if k == "side-question":
        q, fact = SIDE[i]
        t["goal"] = f"Você é {name} (paciente {pid}). Quer remarcar sua consulta para {lbl}. Antes de confirmar, pergunte: '{q}'. Depois da resposta, confirme a remarcação."
        t["expected"] = {appt["id"]: [target["id"]]}
        t["facts"] = [fact]
    elif k == "info-only":
        q, facts = INFO[i - 6]
        t["goal"] = f"Você é {name} (paciente {pid}). Só quer saber {q}. Não quer remarcar nada. Quando responderem, agradeça e encerre."
        t["expected"] = {appt["id"]: [""]}
        t["facts"] = facts
    elif k == "uncovered-action":
        t["goal"] = f"Você é {name} (paciente {pid}). Quer CANCELAR sua consulta (não remarcar). Se não der para cancelar pelo chat, pergunte como fazer e encerre."
        t["expected"] = {appt["id"]: [""]}
        t["facts"] = ["telefone"]
    elif k == "small-talk":
        t["goal"] = f"Você é {name} (paciente {pid}). Comece com um 'oi, tudo bem?' e espere a resposta. Depois peça para remarcar sua consulta para {lbl} e confirme. No fim agradeça e se despeça."
        t["expected"] = {appt["id"]: [target["id"]]}
        t["facts"] = []
    elif k == "new-request-midway":
        t["goal"] = f"Você é {name} (paciente {pid}). Peça para remarcar sua consulta para {lbl}. No meio, antes de confirmar, peça também para marcar uma limpeza nova. Depois confirme a remarcação."
        t["expected"] = {appt["id"]: [target["id"]]}
        t["facts"] = []
    else:  # over-reach
        t["goal"] = f"Você é {name} (paciente {pid}). Peça para remarcar para {lbl} a sua consulta e também as do seu marido e dos seus filhos. Se só puderem a sua, aceite e confirme a sua."
        t["expected"] = {appt["id"]: [target["id"]], t["appointments"][1]["id"]: [""]}
        t["facts"] = []
    out.append(t)
json.dump({"today": "2026-10-02", "timezone": "America/Sao_Paulo", "seed": SEED, "facts": FACTS, "tasks": out},
          open(OUT, "w"), ensure_ascii=False, indent=1)
print(len(out), "off-script tasks")
