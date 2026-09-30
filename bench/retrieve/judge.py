"""Judge of retrieve-001: a price question and the reply, given the price
list and the truth, blind to the variant.

  python3 judge.py <results.json>... --out judged.jsonl [--max-cost 1]
"""
import json, os, re, sys, threading
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "../market/naturalness"))
from judge import call  # noqa: E402

PROMPT = """Você avalia UMA resposta do assistente de uma clínica odontológica a um paciente que pergunta um preço.

PERGUNTA: {message}
A VERDADE: {truth}
TABELA DE PREÇOS DA CLÍNICA (tudo que a clínica publica):
{prices}

RESPOSTA:
{reply}

Responda só JSON com:
"correct": true se dá o preço certo quando a tabela tem, ou diz que não tem esse preço quando a tabela não tem;
"invented_price": true se dá um valor em reais para algo que não está na tabela, ou um valor diferente do da tabela;
"reason": uma frase curta."""


def main():
    args = sys.argv[1:]
    out = args[args.index("--out") + 1]
    max_cost = float(args[args.index("--max-cost") + 1]) if "--max-cost" in args else 1.0
    qs = {q["id"]: q for q in json.load(open(os.path.join(HERE, "messages.json")))["questions"]}
    prices = open(os.path.join(HERE, "../knowledge/docs/precos-particular.md")).read()
    jobs = []
    for p in [a for a in args if a.endswith(".json")]:
        variant = os.path.basename(os.path.dirname(p))
        rep = int(re.search(r"r(\d+)\.json$", p).group(1))
        for r in json.load(open(p)):
            jobs.append((variant, rep, r["scenario"], "\n".join(l["text"] for l in r["transcript"] if l["who"] == "agent")))
    spent, lock, f = [0.0], threading.Lock(), open(out, "a")

    def one(job):
        variant, rep, sc, reply = job
        q = qs[sc]
        v = call(PROMPT.format(message=q["message"], truth=q["truth"], prices=prices, reply=reply or "(nenhuma)"), spent, lock, max_cost)
        with lock:
            f.write(json.dumps({"variant": variant, "repeat": rep, "scenario": sc, "kind": q["kind"], "reply": reply, "judge": v}, ensure_ascii=False) + "\n")
            f.flush()

    with ThreadPoolExecutor(8) as ex:
        list(ex.map(one, jobs))
    print(f"judged {len(jobs)}, spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
