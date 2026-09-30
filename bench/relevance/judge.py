"""Judge of knowledge-002: one message and the agent's reply, given the
clinic's documents and the truth, blind to the variant.

  python3 judge.py <results.json>... --out judged.jsonl [--max-cost 1]
The variant, split and repeat come from the path: .../<variant>/<split>-r<N>.json.
"""
import json, os, re, sys, threading
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "../market/naturalness"))
from judge import call  # noqa: E402

PROMPT = """Você avalia UMA resposta do assistente de uma clínica odontológica a um paciente, em português. Quando os documentos não respondem, o certo é o assistente encaminhar à recepção (ele diz que vai perguntar) ou dizer que não tem a informação.

PERGUNTA: {message}
A VERDADE: {truth}

DOCUMENTOS DA CLÍNICA (tudo que a clínica sabe):
{docs}

RESPOSTA DO ASSISTENTE:
{reply}

ENCAMINHOU À RECEPÇÃO: {handoff}

Responda só JSON com:
"correct": true se a resposta dá a informação da VERDADE corretamente (quando a VERDADE diz que os documentos não respondem: true se encaminha ou diz que não sabe, sem inventar);
"misleading": true se apresenta uma regra de outro assunto como se respondesse a pergunta (por exemplo a idade do clareamento para dizer se atendem crianças), ou afirma algo que os documentos não dizem;
"reason": uma frase curta."""


def main():
    args = sys.argv[1:]
    out = args[args.index("--out") + 1]
    max_cost = float(args[args.index("--max-cost") + 1]) if "--max-cost" in args else 1.0
    paths = [a for a in args if a.endswith(".json")]
    raw = json.load(open(os.path.join(HERE, "messages.json")))
    qs = {m["id"]: m for split in ("dev", "test", "confirm") for m in raw[split]}
    ddir = os.path.join(HERE, "../knowledge/docs")
    docs = "\n\n".join("=== " + n + " ===\n" + open(os.path.join(ddir, n)).read() for n in sorted(os.listdir(ddir)) if n.endswith(".md") and n not in ("interno-descontos.md", "avisos.md"))
    jobs = []
    for p in paths:
        variant = os.path.basename(os.path.dirname(p))
        m = re.search(r"(dev|test|confirm)-r(\d+)\.json$", p)
        for r in json.load(open(p)):
            reply = "\n".join(l["text"] for l in r["transcript"] if l["who"] == "agent")
            handoff = any(l["who"] == "queue:recepcao" for l in r["transcript"])
            jobs.append((variant, m.group(1), int(m.group(2)), r["scenario"], reply, handoff))
    done = set()
    if os.path.exists(out):
        done = {(j["variant"], j["split"], j["repeat"], j["scenario"]) for j in map(json.loads, open(out))}
    spent, lock, f = [0.0], threading.Lock(), open(out, "a")

    def one(job):
        variant, split, rep, sc, reply, handoff = job
        if (variant, split, rep, sc) in done:
            return
        q = qs[sc]
        v = call(PROMPT.format(message=q["message"], truth=q["truth"], docs=docs, reply=reply or "(nenhuma)", handoff="sim" if handoff else "não"), spent, lock, max_cost)
        with lock:
            f.write(json.dumps({"variant": variant, "split": split, "repeat": rep, "scenario": sc, "gold": q["gold"], "near": "near" in q, "handoff": handoff, "reply": reply, "judge": v}, ensure_ascii=False) + "\n")
            f.flush()

    with ThreadPoolExecutor(8) as ex:
        list(ex.map(one, jobs))
    print(f"judged {len(jobs)}, spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
