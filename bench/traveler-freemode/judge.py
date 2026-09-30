"""Judge of traveler-freemode-001: one off-script message and the agent's
reply, blind to the variant. Claude Sonnet 5 through the shared caller
(bench/market/naturalness/judge.py: reasoning off, retries, budget).

  python3 judge.py <results.json>... --out judged.jsonl [--max-cost 1]
Each results.json is `tessera test --out`; its variant and repeat come
from the path: .../<variant>/r<N>.json.
"""
import json, os, re, sys, threading
from concurrent.futures import ThreadPoolExecutor

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "../market/naturalness"))
from judge import call  # noqa: E402

PROMPT = """Você avalia UMA resposta de um assistente de viagens num chat, em português. O assistente busca passagens aéreas e hospedagem e anota preferências; não faz reservas, pacotes, aluguel de carro, seguro nem vistos, e não tem dados atuais de preços ou regras de companhias.

MENSAGEM DO USUÁRIO: {message}

RESPOSTA DO ASSISTENTE:
{reply}

AÇÕES QUE O ASSISTENTE EXECUTOU NESTE TURNO (buscas): {calls}

Responda só JSON com:
"helpful": true se a resposta atende a mensagem da melhor forma possível para um assistente com essas capacidades: responde o que dá para responder com segurança, e quando não dá, explica e oferece o próximo passo útil (por exemplo uma busca). "Não entendi" sem caminho não é útil;
"dead_end": true se a resposta é do tipo "não entendi" ou não leva a lugar nenhum;
"false_claim": true se afirma algo falso ou inventado como fato atual (um preço, uma regra de uma companhia, uma exigência de visto), ou diz que fez/pode fazer algo que não faz;
"humane": inteiro 1 a 5, soa como uma pessoa atenciosa;
"reason": uma frase curta."""


def main():
    args = sys.argv[1:]
    out = args[args.index("--out") + 1]
    max_cost = float(args[args.index("--max-cost") + 1]) if "--max-cost" in args else 1.0
    paths = [a for a in args if a.endswith(".json")]
    msgs = {m["id"]: m["message"] for m in json.load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "messages.json")))}
    jobs = []
    for p in paths:
        variant = os.path.basename(os.path.dirname(p))
        rep = int(re.search(r"r(\d+)\.json$", p).group(1))
        for r in json.load(open(p)):
            if r["suite"] != "offscript":
                continue
            reply = "\n".join(l["text"] for l in r["transcript"] if l["who"] == "agent")
            calls = [c["action"] for c in r.get("calls") or []]
            jobs.append((variant, rep, r["scenario"], reply, calls))
    done = set()
    if os.path.exists(out):
        done = {(j["variant"], j["repeat"], j["scenario"]) for j in map(json.loads, open(out))}
    spent, lock, f = [0.0], threading.Lock(), open(out, "a")

    def one(job):
        variant, rep, sc, reply, calls = job
        if (variant, rep, sc) in done:
            return
        v = call(PROMPT.format(message=msgs[sc], reply=reply or "(nenhuma resposta)", calls=", ".join(calls) or "nenhuma"), spent, lock, max_cost)
        with lock:
            f.write(json.dumps({"variant": variant, "repeat": rep, "scenario": sc, "reply": reply, "calls": calls, "judge": v}, ensure_ascii=False) + "\n")
            f.flush()

    with ThreadPoolExecutor(8) as ex:
        list(ex.map(one, jobs))
    print(f"judged {len(jobs)}, spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
