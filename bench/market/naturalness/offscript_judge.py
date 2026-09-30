"""Off-script judge (docs/research/rfc0004-baseline-preregistration.md,
section 2): per transcript, whether answerable questions got correct
answers, whether the agent made a false claim, whether it hit a dead end
("não entendi" to something it could answer or redirect), and how humane
it was. A run succeeds when the tool server's state was right (from the
harness) and the judge finds it answered, with no false claim and no
dead end.

  python3 offscript_judge.py <runs.jsonl> <tasks-offscript.json> <out-dir> [--max-cost 4]
"""
import json, os, sys, threading, concurrent.futures as cf
sys.path.insert(0, os.path.dirname(__file__))
from judge import call, transcript  # same judge model and client

PROMPT = """Você avalia uma conversa de atendimento de uma clínica odontológica (em português). Um paciente simulado tinha este objetivo:
{goal}

Fatos verdadeiros da clínica (a única fonte de verdade para informações): {facts}
O que de fato mudou no sistema da clínica durante a conversa (efeitos): {effects}
A clínica NÃO tem como cancelar consultas nem marcar consultas novas pelo chat; só remarcar consultas do próprio paciente.

=== Conversa ===
{conv}

Responda SOMENTE um JSON:
- "answered": true se toda pergunta que o atendente PODIA responder (com os fatos acima ou com a remarcação) recebeu resposta correta; false se alguma ficou sem resposta ou com resposta errada; true se não houve pergunta desse tipo
- "false_claim": true se o atendente afirmou algo falso (um fato da clínica errado, ou disse que fez algo que não fez, como cancelar)
- "dead_end": true se o atendente respondeu "não entendi" ou equivalente a algo que ele poderia responder ou redirecionar
- "humane": 1 a 5, quão humano e acolhedor foi o atendimento
- "reason": uma frase curta"""


def main():
    runs_path, tasks_path, out = sys.argv[1], sys.argv[2], sys.argv[3]
    max_cost = float(sys.argv[sys.argv.index("--max-cost") + 1]) if "--max-cost" in sys.argv else 4.0
    tf = json.load(open(tasks_path))
    tasks = {t["id"]: t for t in tf["tasks"]}
    facts = json.dumps(tf["facts"], ensure_ascii=False)
    runs = [json.loads(l) for l in open(runs_path)]
    spent, lock = [0.0], threading.Lock()

    def judge(r):
        v = call(PROMPT.format(goal=tasks[r["task"]]["goal"], facts=facts, effects=json.dumps(r.get("effects"), ensure_ascii=False),
                               conv=transcript(r)), spent, lock, max_cost)
        ok = r["success"] and v.get("answered") is True and not v.get("false_claim") and not v.get("dead_end")
        return {"competitor": r["competitor"], "task": r["task"], "kind": r["kind"], "repeat": r["repeat"], "stateOK": r["success"],
                "judge": v, "success": ok}

    os.makedirs(out, exist_ok=True)
    path = os.path.join(out, "judgments.jsonl")
    done = set()
    if os.path.exists(path):
        for l in open(path):
            x = json.loads(l)
            done.add((x["competitor"], x["task"], x["repeat"]))
    todo = [r for r in runs if (r["competitor"], r["task"], r["repeat"]) not in done]
    n = len(done)
    with open(path, "a") as f, cf.ThreadPoolExecutor(8) as ex:
        for x in ex.map(judge, todo):
            f.write(json.dumps(x, ensure_ascii=False) + "\n")
            f.flush()
            n += 1
            print(n, "/", len(runs), f"${spent[0]:.3f}", file=sys.stderr)
    print(f"spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
