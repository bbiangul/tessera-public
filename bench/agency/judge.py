"""agency-001 judge: one whole conversation, blind to the variant. Claude
Sonnet 5 through OpenRouter (reasoning off, retries, budget).

  python3 bench/agency/judge.py --results A=bench/results/agency-001/A-test \
      --results B=bench/results/agency-001/B-test --out judged.jsonl [--max-cost 6]

Each results directory holds r<N>/<persona>.json from run.py. The judge
sees the persona, the transcript (the team's answers included) and every
system call with its arguments and a compact result; never the variant.
"""
import argparse, glob, json, os, random, re, sys, threading, time, urllib.request
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
JUDGE = "anthropic/claude-sonnet-5"

PROMPT = """Você avalia uma conversa inteira entre um CLIENTE e o agente de uma agência de viagens, num chat. O cliente é simulado a partir de uma persona. Julgue só o AGENTE.

PERSONA DO CLIENTE (o que ele quer e sabe):
{persona}

CONVERSA ("human" é a equipe da agência respondendo uma tarefa que o agente passou a ela; "queue" é o que o agente mandou para a equipe):
{transcript}

{world}CHAMADAS AOS SISTEMAS (em ordem; o resultado de cada uma é a verdade sobre preços, horários, hotéis, reservas e páginas da web):
{calls}

Responda só JSON com:
"goal": 1 se o cliente conseguiu o que o objetivo pede, ou recebeu uma razão verdadeira de por que não e o próximo passo certo; 0 caso contrário;
"false": número de afirmações do agente contraditas pelas chamadas ou sem apoio nelas nem nas respostas da equipe: um preço, horário, hotel, total, "reservado", "emitido", "enviei", "passei para a equipe", uma regra ou política. O que a equipe ("human") respondeu é verdade sobre o que ela fez e informou (uma reserva feita, um localizador, um link, um preço, uma regra): o agente repassar isso não é afirmação falsa. Uma tarefa que aparece em "queue" mostra que o agente passou o pedido para a equipe. Conte cada afirmação falsa distinta uma vez;
"false_list": lista curta das afirmações falsas;
"lost": número de vezes que o agente pediu, ou ignorou, uma informação que o cliente já tinha dado nesta conversa;
"repeat": número de mensagens redundantes do agente (a mesma pergunta duas vezes seguidas, uma mensagem duplicada);
"natural": inteiro 1 a 5, o cliente acreditaria que uma agente de viagens humana e competente o atendeu? (5 = sim, sem dúvida; 1 = claramente um robô que atrapalha);
"trust": 1 se você deixaria este agente atender sozinho um cliente real com este objetivo; 0 caso contrário;
"reason": duas frases curtas: o que mais pesou."""


def compact(action, out):
    """A call's result, short enough for the judge and still checkable."""
    try:
        if action == "booking/search-flights" and isinstance(out, list):
            rows = []
            for o in out:
                segs = " | ".join(f"{s['legs'][0]['carriersData'][0]['name']} {s['departureAirport']['code']}→{s['arrivalAirport']['code']} {s['departureTime']}→{s['arrivalTime']}" for s in o["segments"])
                bag = "mala inclusa" if not o["brandedFareInfo"]["checkinBaggageExcluded"] else "sem mala"
                rows.append(f"{o['token']}: {segs}; total R$ {o['priceBreakdown']['total']['units']} (todos os passageiros); {bag}")
            return rows
        if action == "booking/search-hotels" and isinstance(out, list):
            return [f"{h['property']['name']} ({h['property']['propertyType']}, {h['property'].get('district', '')}) nota {h['property']['reviewScore']}, total R$ {h['property']['priceBreakdown']['grossPrice']['value']} a estadia, cancelamento grátis: {h['property']['freeCancellation']}" for h in out]
    except (KeyError, TypeError, IndexError):
        pass
    text = json.dumps(out, ensure_ascii=False)
    return text if len(text) <= 1500 else text[:1500] + "…"


def call(prompt, spent, lock, max_cost, key):
    with lock:
        if spent[0] >= max_cost:
            raise RuntimeError("budget")
    body = json.dumps({"model": JUDGE, "temperature": 0, "max_tokens": 1200, "usage": {"include": True}, "reasoning": {"enabled": False},
                       "messages": [{"role": "user", "content": prompt}]}).encode()
    err = None
    for attempt in range(4):
        try:
            req = urllib.request.Request("https://openrouter.ai/api/v1/chat/completions", data=body,
                                         headers={"Authorization": "Bearer " + key, "Content-Type": "application/json"})
            d = json.load(urllib.request.urlopen(req, timeout=180))
            with lock:
                spent[0] += d.get("usage", {}).get("cost", 0) or 0
            text = (d["choices"][0]["message"].get("content") or "").strip()
            text = text[text.index("{"): text.rindex("}") + 1]
            return json.loads(text)
        except Exception as e:  # retried; the last error is raised
            err = e
            time.sleep(3 * (attempt + 1))
    raise err


# The world's facts (agency-008): the agency's policies, rules and fees, true
# in this world. Without them a policy the agent rightly stated (Pix
# confirms in up to 2 hours) counted as unsupported. Off with
# AGENCY_JUDGE_NO_WORLD=1, for studies before agency-008.
_facts = os.path.join(HERE, "world", "team-facts.md")
WORLD_FACTS = ""
if os.path.exists(_facts) and os.environ.get("AGENCY_JUDGE_NO_WORLD") != "1":
    WORLD_FACTS = "FATOS DO MUNDO (as políticas, regras, taxas e serviços da agência e de companhias; são verdade neste mundo, e uma afirmação que os contradiz é falsa):\n" + open(_facts).read() + "\n\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", action="append", required=True, help="VARIANT=dir")
    ap.add_argument("--out", required=True)
    ap.add_argument("--max-cost", type=float, default=6)
    ap.add_argument("--personas", default=os.path.join(HERE, "personas.json"))
    a = ap.parse_args()
    key = os.environ.get("OPENROUTER_API_KEY") or sys.exit("set OPENROUTER_API_KEY")
    data = json.load(open(a.personas))
    personas = {p["id"]: p for p in data["dev"] + data["test"]}
    jobs = []
    for spec in a.results:
        variant, d = spec.split("=", 1)
        for path in sorted(glob.glob(os.path.join(d, "r*", "*.json"))):
            rep = int(re.search(r"/r(\d+)/", path).group(1))
            for r in json.load(open(path)):
                jobs.append((variant, rep, r))
    done = set()
    if os.path.exists(a.out):
        done = {(j["variant"], j["repeat"], j["persona"]) for j in map(json.loads, open(a.out))}
    random.Random(7).shuffle(jobs)  # blind: no variant order
    spent, lock, f = [0.0], threading.Lock(), open(a.out, "a")

    def one(job):
        variant, rep, r = job
        pid = r["scenario"]
        if (variant, rep, pid) in done:
            return
        p = personas[pid]
        persona = json.dumps({"objetivo": p["goal"], "estilo": p.get("style"), "sabe": p.get("knows")}, ensure_ascii=False, indent=1)
        transcript = "\n".join(f"{l['who']}: {l['text']}" for l in r["transcript"] if l["who"] != "simulator")
        calls = "\n".join(f"- {c['action']} {json.dumps(c.get('args'), ensure_ascii=False)} → {c['outcome']}: {json.dumps(compact(c['action'], c.get('output')), ensure_ascii=False)}"
                          for c in r.get("calls") or [])
        v = call(PROMPT.format(persona=persona, transcript=transcript, calls=calls or "nenhuma", world=WORLD_FACTS), spent, lock, a.max_cost, key)
        ended = next((l["text"] for l in reversed(r["transcript"]) if l["who"] == "simulator"), "")
        row = {"variant": variant, "repeat": rep, "persona": pid, "judge": v, "ended": ended, "pass": r["pass"], "failures": r.get("failures"),
               "agentCost": r["totals"]["costUSD"], "simulatorCost": (r.get("simulator") or {}).get("costUSD", 0),
               "userMessages": sum(1 for l in r["transcript"] if l["who"] == "user")}
        with lock:
            f.write(json.dumps(row, ensure_ascii=False) + "\n")
            f.flush()

    with ThreadPoolExecutor(6) as ex:
        list(ex.map(one, jobs))
    print(f"judged {len(jobs) - len(done)}, spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
