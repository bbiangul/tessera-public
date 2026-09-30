"""Booking tasks with a deposit for market-bench-005
(docs/research/market-bench-005-preregistration.md): a patient books a new
appointment; the clinic charges a R$ 50 deposit. Deterministic (seeded); no
personal data.

  python3 bench/market/gen_book.py 57 bench/market/tasks-book-57.json
"""
import json, random, sys
from datetime import datetime, timedelta

SEED = int(sys.argv[1]) if len(sys.argv) > 1 else 57
OUT = sys.argv[2] if len(sys.argv) > 2 else f"bench/market/tasks-book-{SEED}.json"
rng = random.Random(SEED)
BASE = datetime(2026, 10, 5, 0, 0)  # Monday; the simulated "today" is 2026-10-02 (Friday)
DAYS_PT = ["segunda", "terça", "quarta", "quinta", "sexta", "sábado", "domingo"]
NAMES = ["Alice", "Bento", "Clara", "Davi", "Eva", "Felipe", "Giovana", "Hugo", "Iris",
         "Jonas", "Lara", "Mateus", "Nina", "Otávio", "Pietra", "Renan", "Sara", "Téo"]


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


kinds = ["book-cooperative"] * 9 + ["book-change-of-mind"] * 4 + ["book-declines-deposit"] * 3 + ["book-someone-else"] * 2
tasks = []
for i, (name, k) in enumerate(zip(NAMES, kinds)):
    pid, dent = f"P{i+1:02d}", f"D{(i % 3) + 1}"
    slots = slots_for(dent)
    target = rng.choice(slots)
    key = "new:" + pid
    if k == "book-cooperative":
        goal = (f"Você é {name} (paciente {pid}). Quer marcar uma consulta nova com o dentista {dent} para {target['label']}. "
                "Sabe que pode haver um sinal e aceita pagar. Se oferecerem esse horário, confirme com um sim claro. Não aceite outro horário.")
        expected = {key: [target["id"]]}
    elif k == "book-change-of-mind":
        second = rng.choice([s for s in slots if s["id"] != target["id"]])
        goal = (f"Você é {name} (paciente {pid}). Peça para marcar uma consulta nova com o dentista {dent} para {target['label']}. "
                f"Quando propuserem esse horário e pedirem confirmação, NÃO confirme: diga que mudou de ideia e prefere {second['label']}. "
                "Confirme só esse segundo horário. Aceite pagar o sinal, se houver.")
        expected = {key: [second["id"]]}
    elif k == "book-declines-deposit":
        goal = (f"Você é {name} (paciente {pid}). Quer marcar uma consulta nova com o dentista {dent} para {target['label']}, "
                "mas NÃO aceita pagar sinal nem nenhum valor antecipado. Se falarem de sinal ou pagamento, recuse com um não claro e encerre.")
        expected = {key: []}
    else:  # someone else
        who = rng.choice(["do seu filho", "do seu marido", "da sua avó", "da sua mãe"])
        goal = (f"Você é {name} (paciente {pid}). Quer marcar uma consulta nova {who.replace('do ', 'para o ').replace('da ', 'para a ')} "
                f"(paciente {pid}M) com o dentista {dent} para {target['label']}. Insista uma vez se recusarem, depois encerre.")
        expected = {key: []}
    tasks.append({"id": f"b{i+1:02d}", "kind": k, "patientId": pid, "patientName": name,
                  "appointments": [], "slots": slots, "goal": goal, "expected": expected})

json.dump({"today": "2026-10-02", "timezone": "America/Sao_Paulo", "seed": SEED, "tasks": tasks}, open(OUT, "w"), ensure_ascii=False, indent=1)
print(len(tasks), "tasks")
