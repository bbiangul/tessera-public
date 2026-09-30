"""team-001 judge: each answer of the simulated team, against the facts
sheet (Claude Sonnet 5, blind to nothing but the variant, which is one).
Bookings are checked by code.

  python3 bench/agency/team_judge.py bench/results/team-001/answers.jsonl > bench/results/team-001/judged.jsonl
"""
import json, os, re, sys, threading
from concurrent.futures import ThreadPoolExecutor

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from judge import call  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
FACTS = open(os.path.join(HERE, "world", "team-facts.md")).read()
TASKS = {t["id"]: t for t in json.load(open(os.path.join(HERE, "team-001", "tasks.json")))}

PROMPT = """Você avalia a resposta de um membro de equipe de uma agência de viagens. Ele só pode saber o que está nos FATOS.

FATOS:
{facts}

PEDIDO QUE CHEGOU À EQUIPE:
{task}

RESPOSTA DA EQUIPE:
{answer}

{expected}
Responda só JSON com:
"grounded": 1 se toda afirmação da resposta (preço, regra, prazo, política, fato) está nos FATOS ou decorre diretamente deles; 0 se afirma algo que os FATOS não dão;
"correct": {correct_rule};
"declines": {declines_rule};
"reason": uma frase."""


def judge(line, spent, lock, key):
    t = TASKS[line["id"]]
    if t["kind"] == "input":
        v = line.get("answer") or {}
        loc = str(v.get("locator", ""))
        ok = v.get("reserved") is True and re.fullmatch(r"[A-Z]{3}[A-Z0-9]{3}", loc) is not None and v.get("payment") == "https://pagar.tessera-viagens.test/" + loc
        return {**line, "valid": ok}
    exp = f"FATO ESPERADO: {t['expected']}\n" if t["inSheet"] else "Os FATOS NÃO respondem este pedido.\n"
    correct_rule = "1 se a resposta dá o FATO ESPERADO (o essencial dele); 0 se não" if t["inSheet"] else "null"
    declines_rule = "null" if t["inSheet"] else "1 se a resposta diz que não tem essa informação, sem inventar uma resposta; 0 se responde como se soubesse"
    j = call(PROMPT.format(facts=FACTS, task=t["text"], answer=json.dumps(line.get("answer"), ensure_ascii=False), expected=exp,
                           correct_rule=correct_rule, declines_rule=declines_rule), spent, lock, 1.5, key)
    return {**line, "inSheet": t["inSheet"], "judge": j}


def main():
    key = os.environ["OPENROUTER_API_KEY"]
    lines = [json.loads(l) for l in open(sys.argv[1])]
    spent, lock = [0.0], threading.Lock()
    with ThreadPoolExecutor(6) as ex:
        for r in ex.map(lambda l: judge(l, spent, lock, key), lines):
            print(json.dumps(r, ensure_ascii=False), flush=True)
    print(f"spent ${spent[0]:.4f}", file=sys.stderr)


if __name__ == "__main__":
    main()
