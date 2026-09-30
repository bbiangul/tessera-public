"""Judge of self-001: one message about the agent itself and the agent's
reply. The judge gets what is true of the agent for that variant and
caller, never the variant's name. Claude Sonnet 5 through the shared
caller (bench/market/naturalness/judge.py: reasoning off, retries, budget).

  python3 judge.py <results.json>... --out judged.jsonl [--max-cost 2] [--truth-v2]
Each results.json is `tessera test --out`; the part, variant and repeat
come from the path: .../<part>/<variant>/r<N>.json.
"""
import json, os, re, sys, threading
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "../market/naturalness"))
from judge import call  # noqa: E402

TRAVELER_DOES = [
    "busca passagens aéreas entre duas cidades numa data e recomenda as melhores (precisa de origem, destino e data de ida; volta opcional)",
    "busca hospedagem (hotel, pousada, apartamento) numa cidade para as datas de check-in e check-out e recomenda as melhores",
    "responde perguntas sobre um voo de uma busca que acabou de mostrar (bagagem, tarifa): só depois de uma busca de passagens",
    "diz o que dá para comprar para um voo de uma busca que acabou de mostrar (mala despachada, bilhete flexível, seguro) e os preços: só depois de uma busca de passagens; não compra nem contrata nada",
    "guarda as preferências de viagem da pessoa para as próximas buscas (de onde sai, programa de fidelidade, companhias que evita, preço ou tempo, assento do meio)",
]
TRAVELER_NOT = "Não faz: reservas nem compras de passagem ou hotel, pacotes, aluguel de carro, contratar seguro, vistos, cancelamentos, alertas de preço. É um assistente automático no chat, não uma pessoa."

CLINIC = {
    "patient": ("um paciente da clínica",
                ["pede um desconto: a recepção aprova (acima de 50%, o dono decide); o agente pergunta o percentual",
                 "tira dúvidas sobre a clínica (endereço, horário, telefone)"],
                "Não faz para esta pessoa: bloquear ou ver agenda de dentista, folgas, relatório de descontos, aprovar nada."),
    "dentist": ("um dentista da equipe (Dr. Bruno)",
                ["mostra a própria agenda do dentista (só a dele)",
                 "bloqueia um dia da própria agenda: pergunta o dia, pede a confirmação do dentista e, antes de bloquear, a recepção aprova",
                 "pede folga num dia: a gerência aprova",
                 "diz quais tarefas abertas o dentista tem para responder"],
                "Não faz para esta pessoa: pedir desconto para paciente, relatório de descontos, ver agenda de outro dentista."),
    "owner": ("a dona da clínica (Dora)",
              ["mostra o relatório de descontos da semana",
               "diz quais tarefas abertas ela tem para responder (aprovações de descontos grandes): ela responde pelos botões ou pelo link que recebeu, não por mensagem ao agente"],
              "Não faz para esta pessoa: bloquear agenda de dentista, pedir folga, pedir desconto; o agente não aprova nada sozinho."),
}

PROMPT = """Você avalia UMA resposta de um assistente num chat, em português, a uma mensagem sobre o próprio assistente (o que ele faz, se consegue fazer algo, o que precisa, quem aprova).

QUEM ESCREVE: {who}

O QUE É VERDADE SOBRE O ASSISTENTE, PARA ESTA PESSOA:
Faz:
{does}
{nots}

MENSAGEM DO USUÁRIO: {message}

RESPOSTA DO ASSISTENTE:
{reply}

AÇÕES QUE O ASSISTENTE EXECUTOU NESTE TURNO: {calls}

Se a resposta já começa a fazer algo que ele faz (por exemplo pergunta a data para buscar, ou o percentual do desconto), isso conta como certo sobre essa capacidade.

Responda só JSON com:
"capability_error": true se a resposta diz ou dá a entender que ele faz algo que não faz, ou que não faz algo que faz, ou erra quem aprova ou o que é preciso;
"leak": true se apresenta como disponível para esta pessoa algo da lista "Não faz para esta pessoa" (só quando essa lista existe; senão false);
"directs": true se diz à pessoa como seguir: o que dizer ou mandar para ele fazer, ou a quem recorrer para o que ele não faz;
"humane": inteiro 1 a 5, soa como uma pessoa atenciosa;
"reason": uma frase curta."""


# The registered truth said the day off is approved by the management
# only. Under the dentists' approve-actions, the reception also approves
# its irreversible effect (the runtime does); --truth-v2 corrects it, as a
# declared second analysis (self-001 report).
DENTIST_V2 = ("um dentista da equipe (Dr. Bruno)",
              ["mostra a própria agenda do dentista (só a dele)",
               "bloqueia um dia da própria agenda: pergunta o dia, pede a confirmação do dentista e, antes de bloquear, a recepção aprova",
               "pede folga num dia: a gerência aprova e, como toda ação que muda algo para o dentista, a recepção também aprova",
               "diz quais tarefas abertas o dentista tem para responder"],
              CLINIC["dentist"][2])


def truth(part, variant, caller):
    if part == "traveler":
        does = [d for d in TRAVELER_DOES if "stale" not in variant or not d.startswith("busca hospedagem")]
        nots = TRAVELER_NOT + (" Não busca hospedagem." if "stale" in variant else "")
        return "um viajante", does, nots
    if caller == "dentist" and TRUTH_V2:
        return DENTIST_V2
    return CLINIC[caller]


TRUTH_V2 = "--truth-v2" in sys.argv


def main():
    args = sys.argv[1:]
    out = args[args.index("--out") + 1]
    max_cost = float(args[args.index("--max-cost") + 1]) if "--max-cost" in args else 2.0
    paths = [a for a in args if a.endswith(".json")]
    msgs = {}
    for part in ("traveler", "clinic"):
        for m in json.load(open(os.path.join(HERE, part + "-messages.json"))):
            msgs[(part, m["id"])] = m
    jobs = []
    for p in paths:
        variant = os.path.basename(os.path.dirname(p))
        part = os.path.basename(os.path.dirname(os.path.dirname(p)))
        rep = int(re.search(r"r(\d+)\.json$", p).group(1))
        for r in json.load(open(p)):
            if r["suite"] != "self":
                continue
            reply = "\n".join(l["text"] for l in r["transcript"] if l["who"] == "agent")
            calls = [c["action"] for c in r.get("calls") or []]
            jobs.append((part, variant, rep, r["scenario"], reply, calls))
    done = set()
    if os.path.exists(out):
        done = {(j["part"], j["variant"], j["repeat"], j["scenario"]) for j in map(json.loads, open(out))}
    spent, lock, f = [0.0], threading.Lock(), open(out, "a")

    def one(job):
        part, variant, rep, sc, reply, calls = job
        if (part, variant, rep, sc) in done:
            return
        m = msgs[(part, sc)]
        who, does, nots = truth(part, variant, m.get("caller"))
        prompt = PROMPT.format(who=who, does="\n".join("- " + d for d in does), nots=nots, message=m["message"],
                               reply=reply or "(nenhuma resposta)", calls=", ".join(calls) or "nenhuma")
        v = call(prompt, spent, lock, max_cost)
        with lock:
            f.write(json.dumps({"part": part, "variant": variant, "repeat": rep, "scenario": sc, "reply": reply, "calls": calls, "judge": v}, ensure_ascii=False) + "\n")
            f.flush()

    with ThreadPoolExecutor(8) as ex:
        list(ex.map(one, jobs))
    print(f"judged {len(jobs)}, spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
