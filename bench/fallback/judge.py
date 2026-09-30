"""Judge of fallback-001: one question and the reply, given every
document and the truth, blind to the variant.

  python3 judge.py <results.json>... --out judged.jsonl [--max-cost 1]
Paths: .../<variant>/r<N>.json.
"""
import json, os, re, sys, threading
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "../market/naturalness"))
from judge import call  # noqa: E402

PROMPT = """Você avalia UMA resposta do assistente de uma clínica odontológica a um paciente, em português.

PERGUNTA: {message}
A VERDADE: {truth}

DOCUMENTOS DA CLÍNICA (tudo que a clínica sabe):
{docs}

RESPOSTA:
{reply}

Responda só JSON com:
"overreach": true se a resposta conclui sim, não ou um valor para o caso específico perguntado quando os documentos não resolvem esse caso (estende uma regra de um caso vizinho a um caso que os documentos não nomeiam); false se diz o que os documentos dizem e que alguém confirma o caso, ou se os documentos de fato resolvem o caso;
"correct": true se a resposta está de acordo com a VERDADE;
"not_understood": true se a resposta é do tipo "não entendi" / "desculpe, não entendi";
"reason": uma frase curta."""


def main():
    args = sys.argv[1:]
    out = args[args.index("--out") + 1]
    max_cost = float(args[args.index("--max-cost") + 1]) if "--max-cost" in args else 1.0
    raw = json.load(open(os.path.join(HERE, "messages.json")))
    qs = {m["id"]: dict(m, kind="control" if s == "controls" else "trap") for s in ("controls", "traps") for m in raw[s]}
    ddir = os.path.join(HERE, "../knowledge/docs")
    docs = "\n\n".join("=== " + n + " ===\n" + open(os.path.join(ddir, n)).read() for n in sorted(os.listdir(ddir)) if n.endswith(".md") and n not in ("interno-descontos.md", "avisos.md"))
    jobs = []
    for p in [a for a in args if a.endswith(".json")]:
        variant = os.path.basename(os.path.dirname(p))
        m = re.search(r"r(\d+)\.json$", p)
        for r in json.load(open(p)):
            jobs.append((variant, "test", int(m.group(1)), r["scenario"], "\n".join(l["text"] for l in r["transcript"] if l["who"] == "agent")))
    done = set()
    if os.path.exists(out):
        done = {(j["variant"], j["split"], j["repeat"], j["scenario"]) for j in map(json.loads, open(out))}
    spent, lock, f = [0.0], threading.Lock(), open(out, "a")

    def one(job):
        variant, split, rep, sc, reply = job
        if (variant, split, rep, sc) in done:
            return
        q = qs[sc]
        v = call(PROMPT.format(message=q["message"], truth=q["truth"], docs=docs, reply=reply or "(nenhuma)"), spent, lock, max_cost)
        with lock:
            f.write(json.dumps({"variant": variant, "split": split, "repeat": rep, "scenario": sc, "kind": q["kind"], "reply": reply, "judge": v}, ensure_ascii=False) + "\n")
            f.flush()

    with ThreadPoolExecutor(8) as ex:
        list(ex.map(one, jobs))
    print(f"judged {len(jobs)}, spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
