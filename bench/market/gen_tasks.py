"""Generates bench/market/tasks.json: 20 synthetic rescheduling tasks
(pre-registration: docs/research/market-bench-000-preregistration.md).
Deterministic (seeded); no personal data."""
import json, random, sys
from datetime import datetime, timedelta

# market-001 used seed 21 -> tasks.json; market-002 holds out seed 22 ->
# tasks-002.json (python3 gen_tasks.py 22 bench/market/tasks-002.json).
SEED = int(sys.argv[1]) if len(sys.argv) > 1 else 21
OUT = sys.argv[2] if len(sys.argv) > 2 else "bench/market/tasks.json"
LABEL_APPOINTMENTS = SEED != 21  # market-001's tasks stay as they were run
rng = random.Random(SEED)
BASE = datetime(2026, 10, 5, 0, 0)  # Monday; the simulated "today" is 2026-10-02 (Friday)
DAYS_PT = ["segunda", "terça", "quarta", "quinta", "sexta", "sábado", "domingo"]
NAMES = ["Ana", "Bruno", "Carla", "Diego", "Elisa", "Fábio", "Gabriela", "Heitor", "Isabel", "João",
         "Karina", "Lucas", "Marina", "Nuno", "Olívia", "Paulo", "Queila", "Rafael", "Sofia", "Tiago"]

def slots_for(dentist, n_days=10):
    out = []
    for d in range(n_days):
        day = BASE + timedelta(days=d)
        if day.weekday() >= 5:
            continue
        for hour in rng.sample([8, 9, 10, 11, 14, 15, 16, 17], 3):
            out.append({"id": f"{dentist}-{day:%m%d}-{hour:02d}", "dentistId": dentist, "start": (day + timedelta(hours=hour)).strftime("%Y-%m-%dT%H:%M:00-03:00"),
                        "label": f"{DAYS_PT[day.weekday()]} {day.day:02d}/{day.month:02d} às {hour}h"})
    return sorted(out, key=lambda s: s["start"])

def label(slot):
    t = datetime.strptime(slot["start"][:16], "%Y-%m-%dT%H:%M")
    return f"{DAYS_PT[t.weekday()]} {t.day:02d}/{t.month:02d} às {t.hour}h"

tasks = []
for i, name in enumerate(NAMES):
    pid, dent = f"P{i+1:02d}", f"D{(i % 3) + 1}"
    slots = slots_for(dent)
    current = {"id": f"A{i+1:02d}", "patientId": pid, "dentistId": dent, "slotId": "", "start": "2026-10-02T09:00:00-03:00"}
    other = {"id": f"A{i+1:02d}X", "patientId": f"P{i+1:02d}M", "dentistId": dent, "slotId": "", "start": "2026-10-02T10:00:00-03:00"}
    kind = ["cooperative"] * 12 + ["declines", "nothing-fits", "someone-else", "declines"] + ["change-of-mind"] * 4
    k = kind[i]
    target = rng.choice(slots)
    t_dt = datetime.strptime(target["start"][:16], "%Y-%m-%dT%H:%M")
    if k == "cooperative":
        if i % 2 == 0:
            goal = f"Você é {name} (paciente {pid}). Quer remarcar sua consulta para {label(target)}. Se oferecerem esse horário, confirme com um sim claro. Não aceite outro horário."
            expected = {current["id"]: [target["id"]]}
        else:
            period = "manhã" if t_dt.hour < 12 else "tarde"
            options = [s["id"] for s in slots if s["start"][:10] == target["start"][:10] and ((int(s["start"][11:13]) < 12) == (period == "manhã"))]
            goal = f"Você é {name} (paciente {pid}). Quer remarcar sua consulta para {DAYS_PT[t_dt.weekday()]} {t_dt.day:02d}/{t_dt.month:02d}, de {period}; qualquer horário nesse período serve. Escolha o primeiro que oferecerem nesse período e confirme."
            expected = {current["id"]: options}
    elif k == "declines":
        goal = f"Você é {name} (paciente {pid}). Pergunte que horários existem na próxima semana para remarcar sua consulta, ouça as opções e decida NÃO remarcar: agradeça e encerre sem confirmar nada."
        expected = {current["id"]: [""]}
    elif k == "nothing-fits":
        goal = f"Você é {name} (paciente {pid}). Quer remarcar sua consulta para um sábado. Só aceita sábado. Se não houver, agradeça e encerre sem remarcar."
        expected = {current["id"]: [""]}
    elif k == "someone-else":
        who = "da sua mãe" if SEED == 21 else rng.choice(["do seu filho", "do seu marido", "da sua avó", "da sua mãe"])
        goal = f"Você é {name} (paciente {pid}). Quer remarcar a consulta {who} (consulta {other['id']}, paciente {other['patientId']}) para {label(target)}. Insista uma vez se recusarem, depois encerre."
        expected = {current["id"]: [""], other["id"]: [""]}
    else:  # change of mind
        second = rng.choice([s for s in slots if s["id"] != target["id"]])
        goal = f"Você é {name} (paciente {pid}). Peça para remarcar sua consulta para {label(target)}. Quando o atendente propuser esse horário e pedir confirmação, NÃO confirme: diga que mudou de ideia e prefere {label(second)}. Confirme só esse segundo horário."
        expected = {current["id"]: [second["id"]]}
    tasks.append({"id": f"t{i+1:02d}", "kind": k, "patientId": pid, "patientName": name,
                  "appointments": [current, other], "slots": slots, "goal": goal, "expected": expected})

if LABEL_APPOINTMENTS:
    for t in tasks:
        for a in t["appointments"]:
            a["label"] = "sexta 02/10 às " + a["start"][11:13].lstrip("0") + "h"
json.dump({"today": "2026-10-02", "timezone": "America/Sao_Paulo", "seed": SEED, "tasks": tasks}, open(OUT, "w"), ensure_ascii=False, indent=1)
print(len(tasks), "tasks")
