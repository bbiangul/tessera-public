"""Blind naturalness judge (docs/research/rfc0004-baseline-preregistration.md).

Pairs Tessera's transcript with each competitor's on the same task and
repeat, asks a judge of another model family which sounds more natural,
is clearer and more efficient, and whether either states a false fact
against the tool server's ground truth. Each pair is judged twice with
the sides swapped; a verdict counts only when both orders agree.

  python3 judge.py <runs.jsonl> <tasks.json> <out-dir> [--max-cost 8] [--limit N]
"""
import json, os, random, sys, time, urllib.request, concurrent.futures as cf

JUDGE = "anthropic/claude-sonnet-5"
KEY = os.environ["OPENROUTER_API_KEY"]
PROMPT = """Você avalia duas conversas de atendimento de uma clínica odontológica (em português). Nas duas, um paciente simulado tenta o mesmo objetivo com um atendente diferente (A e B). Você não sabe quem é quem.

Objetivo do paciente: {goal}

Verdade do sistema da clínica ao final (use para checar fatos): {truth}

=== Conversa A ===
{a}

=== Conversa B ===
{b}

Responda SOMENTE um JSON com estas chaves:
- "natural": "A", "B" ou "empate" — qual atendente soa mais como uma boa recepcionista humana
- "clear": "A", "B" ou "empate" — qual é mais fácil de entender e seguir
- "efficient": "A", "B" ou "empate" — qual chega ao objetivo com menos passos desnecessários
- "factual_error_A": true/false — A afirma algo falso frente à verdade (data, horário, o que foi feito)?
- "factual_error_B": true/false
- "reason": uma frase curta"""


def transcript(r):
    out = []
    for t in r["transcript"]:
        if t["who"] == "patient":
            out.append("Paciente: " + t["text"])
        elif t["who"] == "agent":
            out.append("Atendente: " + t["text"])
    return "\n".join(out)


def call(prompt, spent, lock, max_cost):
    with lock:
        if spent[0] >= max_cost:
            raise RuntimeError("budget")
    # Reasoning off: with it on, the judge spent the whole budget thinking
    # and returned no answer (finish_reason length, empty content).
    body = json.dumps({"model": JUDGE, "temperature": 0, "max_tokens": 800, "usage": {"include": True}, "reasoning": {"enabled": False},
                       "messages": [{"role": "user", "content": prompt}]}).encode()
    for attempt in range(4):
        try:
            req = urllib.request.Request("https://openrouter.ai/api/v1/chat/completions", data=body,
                                         headers={"Authorization": "Bearer " + KEY, "Content-Type": "application/json"})
            d = json.load(urllib.request.urlopen(req, timeout=120))
            with lock:
                spent[0] += d.get("usage", {}).get("cost", 0) or 0
            text = (d["choices"][0]["message"].get("content") or "").strip()
            if "{" not in text:
                raise ValueError("empty or non-JSON reply")
            text = text[text.index("{"): text.rindex("}") + 1]
            return json.loads(text)
        except Exception as e:  # retried; the last error is raised
            err = e
            time.sleep(2 * (attempt + 1))
    raise err


def main():
    runs_path, tasks_path, out = sys.argv[1], sys.argv[2], sys.argv[3]
    max_cost = float(sys.argv[sys.argv.index("--max-cost") + 1]) if "--max-cost" in sys.argv else 8.0
    limit = int(sys.argv[sys.argv.index("--limit") + 1]) if "--limit" in sys.argv else None
    goals = {t["id"]: t["goal"] for t in json.load(open(tasks_path))["tasks"]}
    runs = [json.loads(l) for l in open(runs_path)]
    c0 = {(r["competitor"], r["task"], r["repeat"]): r for r in runs if r["condition"] == "C0"}
    pairs = [(c0[k], c0[("tessera",) + k[1:]]) for k in sorted(c0) if k[0] != "tessera" and ("tessera",) + k[1:] in c0]
    if limit:
        pairs = pairs[:limit]
    os.makedirs(out, exist_ok=True)
    spent, lock = [0.0], __import__("threading").Lock()

    def judge(pair):
        other, tess = pair
        truth_t = json.dumps({"efeitos_tessera": tess.get("effects"), "efeitos_outro": other.get("effects")}, ensure_ascii=False)
        res = {"task": tess["task"], "repeat": tess["repeat"], "competitor": other["competitor"]}
        # Order 1: Tessera is A. Order 2: Tessera is B.
        for order, (a, b) in (("TA", (tess, other)), ("TB", (other, tess))):
            v = call(PROMPT.format(goal=goals[tess["task"]], truth=truth_t, a=transcript(a), b=transcript(b)), spent, lock, max_cost)
            res[order] = v
        return res

    # Each verdict is written as it comes; a rerun skips the pairs done.
    path = os.path.join(out, "judgments.jsonl")
    done = set()
    if os.path.exists(path):
        for l in open(path):
            r = json.loads(l)
            done.add((r["competitor"], r["task"], r["repeat"]))
    todo = [p for p in pairs if (p[0]["competitor"], p[0]["task"], p[0]["repeat"]) not in done]
    n = len(done)
    with open(path, "a") as f, cf.ThreadPoolExecutor(8) as ex:
        for r in ex.map(judge, todo):
            f.write(json.dumps(r, ensure_ascii=False) + "\n")
            f.flush()
            n += 1
            print(n, "/", len(pairs), f"${spent[0]:.3f}", file=sys.stderr)
    print(f"spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
