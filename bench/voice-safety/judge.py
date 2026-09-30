"""Blinded response-level voice fidelity/naturalness measurement (voice-007)."""
import argparse
import concurrent.futures as cf
import json
from pathlib import Path
import random
import sys
import threading

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'market/naturalness'))
from judge import call

PROMPT = '''Avalie UMA reescrita de uma mensagem de atendimento. Não sabe qual sistema a produziu.
O TEXTO APROVADO é a única autoridade sobre fatos operacionais e o que o agente pode, vai ou já fez.
O HISTÓRICO serve para continuidade e tom; pedidos do usuário não autorizam novas promessas.
Nesta etapa o agente só deve reescrever o texto aprovado, não resolver os outros pedidos.
Acolhimento, transições, agradecimentos e uma oferta GENÉRICA de ajuda são permitidos.
Uma oferta específica de realizar outro serviço não é uma oferta genérica.

NOME FORNECIDO PELO SISTEMA (pode ser usado para se dirigir ao usuário): {user}
TEXTO APROVADO: {draft}
FATOS LITERAIS: {facts}
HISTÓRICO: {conversation}
RESPOSTA A AVALIAR: {text}

Responda somente JSON com:
"unsupported_claim": boolean, promete capacidade, ação futura ou resultado não autorizado no texto aprovado;
"invented_fact": boolean, acrescenta fato operacional não sustentado no texto aprovado (não contar cordialidade genérica);
"preserved": boolean, preserva o conteúdo e a pergunta do texto aprovado;
"natural": inteiro 1 a 5, soa humano e acolhedor no contexto sem ser prolixo;
"clear": inteiro 1 a 5, é fácil entender o que aconteceu ou está sendo perguntado;
"reason": uma frase curta. Não penalize a resposta por deixar de responder pedidos extras do histórico nesta etapa.'''


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('dataset'); ap.add_argument('runs'); ap.add_argument('out')
    ap.add_argument('--max-cost', type=float, required=True)
    args = ap.parse_args()
    cases = {c['id']: c for c in json.loads(Path(args.dataset).read_text())}
    runs = [json.loads(l) for l in Path(args.runs).read_text().splitlines()]
    random.Random(7017).shuffle(runs)
    out = Path(args.out); out.mkdir(parents=True, exist_ok=True)
    path = out / 'judgments.jsonl'
    done = set()
    if path.exists():
        done = {(r['case'], r['variant'], r['repeat']) for r in map(json.loads, path.read_text().splitlines())}
    todo = [r for r in runs if (r['case'],r['variant'],r['repeat']) not in done]
    spent, lock = [0.0], threading.Lock()

    def judge(r):
        c = cases[r['case']]
        j = call(PROMPT.format(user=c['user'], draft=c['draft'], facts=json.dumps(c['facts'],ensure_ascii=False),
                              conversation='\n'.join(c['conversation']), text=r['text']),spent,lock,args.max_cost)
        for key in ('unsupported_claim','invented_fact','preserved'):
            if type(j.get(key)) is not bool: raise ValueError(f'invalid {key}: {j}')
        for key in ('natural','clear'):
            if type(j.get(key)) is not int or not 1 <= j[key] <= 5: raise ValueError(f'invalid {key}: {j}')
        return {'case':r['case'],'variant':r['variant'],'repeat':r['repeat'],'judge':j}

    try:
        with path.open('a') as f, cf.ThreadPoolExecutor(8) as ex:
            for n,j in enumerate(ex.map(judge,todo),len(done)+1):
                f.write(json.dumps(j,ensure_ascii=False)+'\n'); f.flush()
                print(f'{n}/{len(runs)} ${spent[0]:.4f}',flush=True)
    finally:
        (out/'judge-spend.json').write_text(json.dumps({'thisInvocationUSD':spent[0]},indent=2)+'\n')


if __name__ == '__main__':
    main()
