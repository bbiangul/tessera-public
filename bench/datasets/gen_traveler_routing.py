#!/usr/bin/env python3
"""Writes traveler-routing-v1.jsonl: synthetic messages for the traveler's
router, labelled with the Function they must reach (or none).

Synthetic, written for the benchmark (no customer data). Messages are
written the way people type in chat: no accents, abbreviations, typos.
Some carry the agent's last message, where the context decides: a short
question after a list of flights is about those flights; the same words
with nothing before are not.

Run: python3 bench/datasets/gen_traveler_routing.py > bench/datasets/traveler-routing-v1.jsonl
     python3 bench/datasets/gen_traveler_routing.py --holdout > bench/datasets/traveler-routing-holdout-v1.jsonl

     python3 bench/datasets/gen_traveler_routing.py --holdout2 > bench/datasets/traveler-routing-holdout-v2.jsonl
     python3 bench/datasets/gen_traveler_routing.py --holdout3 > bench/datasets/traveler-routing-holdout-v3.jsonl
     python3 bench/datasets/gen_traveler_routing.py --v2 > bench/datasets/traveler-routing-v2.jsonl
     python3 bench/datasets/gen_traveler_routing.py --holdout4 > bench/datasets/traveler-routing-holdout-v4.jsonl

v2 is v1 plus flight-extras (tuning); holdout-v4 was written with it,
before any extras text was tuned, and is only measured.

Each holdout was written before the texts it measures were tuned: v1
before tuning on v1 (holdout-v1 then became tuning data for the hotels
text), holdout-v2 before that. docs/research/router-002.md has the history.
"""
import sys
import json

FLIGHTS_SHOWN = (
    "Encontrei estas opções:\n"
    "1. GOL GRU → REC, 12/10 12:40–15:55 (direto, 3h15) — R$ 640\n"
    "2. Azul GRU → REC, 12/10 16:05–19:30 (direto, 3h25) — R$ 720, mala despachada inclusa\n"
    "3. LATAM GRU → REC, 12/10 07:10–10:25 (direto, 3h15) — R$ 890"
)
HOTELS_SHOWN = (
    "Hotéis em Lisboa, 12/10–15/10 (3 noites):\n"
    "1. Hotel Avenida Palace — nota 9.0 (2100 avaliações), R$ 3.000 no total (R$ 1.000/noite)\n"
    "2. Lisboa Stay Apartments — nota 8.8 (900 avaliações), R$ 1.800 no total (R$ 600/noite), cancelamento grátis"
)
SAVED = "Anotado para as próximas buscas: você costuma sair de São Paulo."

CASES = {
    "search-flights": [
        ("quero viajar", None),
        ("Quero voar de São Paulo para Recife dia 12 de outubro", None),
        ("tem voo pra lisboa amanha?", None),
        ("passagem sp rio sexta que vem", None),
        ("preciso ir pra Brasilia semana que vem, volto na quinta", None),
        ("quanto ta uma passagem pra salvador em novembro?", None),
        ("me acha um voo barato pra fortaleza", None),
        ("vou pra NY no natal com minha esposa e 2 filhos", None),
        ("business pra Paris dia 3/11", None),
        ("voo de manha pra BH amanha, preciso chegar ate 11h", None),
        ("queria ir pra floripa no feriado", None),
        ("ida e volta POA 10 a 15 de dezembro", None),
        ("to pensando em viajar pro nordeste, o que tem de voo?", None),
        ("preciso de passagem urgente pra Manaus hoje a noite", None),
        ("e se eu for pra Lisboa em vez disso?", FLIGHTS_SHOWN),
        ("agora quero ver voos pra Salvador no dia 20", FLIGHTS_SHOWN),
        ("e pra volta, dia 19, tem o que?", FLIGHTS_SHOWN),
        ("beleza, agora quero ir pra Recife dia 12", SAVED),
        ("bora ver passagem pra lisboa", HOTELS_SHOWN),
        ("voos pro Chile em janeiro", None),
    ],
    "flight-details": [
        ("e o da LATAM?", FLIGHTS_SHOWN),
        ("o segundo tem mala?", FLIGHTS_SHOWN),
        ("e a bagagem do primeiro?", FLIGHTS_SHOWN),
        ("qual a tarifa da azul?", FLIGHTS_SHOWN),
        ("o mais barato inclui mala despachada?", FLIGHTS_SHOWN),
        ("esse da gol é light?", FLIGHTS_SHOWN),
        ("posso levar mala de mao no 3?", FLIGHTS_SHOWN),
        ("o de 640 tem bagagem?", FLIGHTS_SHOWN),
        ("e o ultimo?", FLIGHTS_SHOWN),
        ("me fala mais do voo da azul", FLIGHTS_SHOWN),
        ("o primeiro tem despachada inclusa?", FLIGHTS_SHOWN),
        ("qual deles inclui mala?", FLIGHTS_SHOWN),
        ("e a tarifa do segundo?", FLIGHTS_SHOWN),
        ("detalhes do voo das 7h10", FLIGHTS_SHOWN),
        ("o da latam tem mala?", FLIGHTS_SHOWN),
    ],
    "search-hotels": [
        ("Preciso de hotel em Lisboa de 12 a 15 de outubro", None),
        ("preciso de um lugar pra ficar", None),
        ("tem hotel bom perto da paulista?", None),
        ("quero um airbnb em floripa pro carnaval", None),
        ("hospedagem em Recife dia 12 ate 14", None),
        ("pousada em paraty no fim de semana", None),
        ("vou ficar um mes em lisboa, onde fico?", None),
        ("hotel barato em salvador semana que vem", None),
        ("me indica um hotel em Paris com cafe da manha", None),
        ("e hotel em Recife?", FLIGHTS_SHOWN),
        ("agora preciso de hotel la pra essas datas", FLIGHTS_SHOWN),
        ("e onde eu fico em recife?", FLIGHTS_SHOWN),
        ("tem apartamento em vez de hotel?", HOTELS_SHOWN),
        ("e hotel mais barato que esses?", HOTELS_SHOWN),
        ("quero reservar quarto pra 2 adultos em BH", None),
    ],
    "set-preferences": [
        ("Moro em São Paulo, costumo sair de lá", None),
        ("prefiro evitar a GOL", None),
        ("sou Smiles Diamante", None),
        ("odeio assento do meio", None),
        ("pra mim o que importa é sempre o menor preço", None),
        ("prefiro chegar rapido mesmo pagando mais", None),
        ("wifi ruim em hotel é inaceitavel pra mim", None),
        ("tenho LATAM Pass Gold", None),
        ("nunca me coloca na azul", None),
        ("costumo sair do Rio, do Galeão", None),
        ("anota ai: detesto barulho em hotel", None),
        ("a partir de agora prefiro sempre voos mais baratos", FLIGHTS_SHOWN),
        ("nao gosto da gol, lembra disso", FLIGHTS_SHOWN),
        ("sou de curitiba", None),
        ("minha prioridade é equilibrio entre preço e tempo", None),
    ],
    "none": [
        ("bom dia!", None),
        ("obrigado!", FLIGHTS_SHOWN),
        ("valeu, era isso", HOTELS_SHOWN),
        ("qual a previsao do tempo em lisboa?", None),
        ("preciso de visto pra ir pros EUA?", None),
        ("quanto custa um seguro viagem?", None),
        ("aluguel de carro em orlando", None),
        ("cancela minha reserva", None),
        ("qual o horario da farmacia?", None),
        ("me conta uma piada", None),
        ("vc é um robo?", None),
        ("quero falar com um humano", None),
        ("qual a cotacao do euro hoje?", None),
        ("o que levar na mala pra europa no inverno?", None),
        ("ok", FLIGHTS_SHOWN),
    ],
}


HOLDOUT = {
    "search-flights": [
        ("oi, quero comprar uma passagem", None),
        ("gru x cnf dia 4", None),
        ("tem aviao pra natal no sabado?", None),
        ("preciso estar em brasilia segunda 9h", None),
        ("quero levar minha mae pra lisboa em marco", None),
        ("e pra buenos aires, quanto fica?", FLIGHTS_SHOWN),
        ("preciso de voo de volta pra sp dia 15", HOTELS_SHOWN),
        ("voo direto pra miami em julho", None),
    ],
    "flight-details": [
        ("o da gol tem mala?", FLIGHTS_SHOWN),
        ("e a do segundo, despacha?", FLIGHTS_SHOWN),
        ("o terceiro é qual tarifa?", FLIGHTS_SHOWN),
        ("no de 720 posso despachar?", FLIGHTS_SHOWN),
        ("e esse primeiro ai, inclui o que?", FLIGHTS_SHOWN),
        ("o voo das 16h tem bagagem?", FLIGHTS_SHOWN),
    ],
    "search-hotels": [
        ("onde posso dormir em ouro preto sabado?", None),
        ("hostel em BA pra 5 noites", None),
        ("flat em sp pra 2 semanas", None),
        ("e hospedagem em lisboa pra essas datas?", FLIGHTS_SHOWN),
        ("tem algo com cancelamento gratis?", HOTELS_SHOWN),
        ("resort em porto de galinhas no ano novo", None),
    ],
    "set-preferences": [
        ("tenho tudoazul", None),
        ("nao voo de avianca de jeito nenhum", None),
        ("sempre saio de campinas", None),
        ("prefiro corredor, nunca meio", None),
        ("tempo é mais importante que preço pra mim", None),
        ("academia no hotel é essencial pra mim", None),
    ],
    "none": [
        ("boa noite", None),
        ("qual o fuso de lisboa?", None),
        ("precisa de vacina pra ir pra africa do sul?", None),
        ("vcs vendem pacote de cruzeiro?", None),
        ("perfeito, obrigado", FLIGHTS_SHOWN),
        ("que horas sao ai?", None),
        ("quero reclamar de uma cobranca", None),
        ("me manda o comprovante", None),
    ],
}


HOLDOUT2 = {
    "search-flights": [
        ("quanto ta pra ir pra recife?", None),
        ("passagem pra mim e meu marido pra roma", None),
        ("to em lisboa e preciso voltar pro brasil dia 30", None),
        ("vcs acham voo pra cancun?", None),
        ("e saindo de campinas?", FLIGHTS_SHOWN),
        ("sdu cgh amanha cedo", None),
    ],
    "flight-details": [
        ("a azul inclui despachada?", FLIGHTS_SHOWN),
        ("o de 890 tem o que de bagagem?", FLIGHTS_SHOWN),
        ("e o segundo, qual tarifa?", FLIGHTS_SHOWN),
        ("esse das 12h40 é light?", FLIGHTS_SHOWN),
    ],
    "search-hotels": [
        ("casa de temporada em buzios no reveillon", None),
        ("quero um quarto em curitiba amanha", None),
        ("apart hotel em sp pro mes que vem", None),
        ("chale em campos do jordao pro inverno", None),
        ("e onde eu durmo la?", FLIGHTS_SHOWN),
        ("tem algum com nota melhor?", HOTELS_SHOWN),
    ],
    "set-preferences": [
        ("sou smiles ouro", None),
        ("latam nunca mais", None),
        ("moro em recife", None),
        ("gosto de janela", None),
        ("barulho me incomoda muito em hotel", None),
    ],
    "none": [
        ("tudo bem?", None),
        ("qual o melhor bairro de lisboa pra turista?", None),
        ("como faço check-in online na gol?", None),
        ("meu voo atrasou, o que eu faço?", None),
        ("show, valeu", HOTELS_SHOWN),
        ("vcs tem cupom de desconto?", None),
    ],
}


HOLDOUT3 = {
    "search-flights": [
        ("me ve uma passagem pra goiania", None),
        ("quero ir pra santiago com 2 criancas em julho", None),
        ("tem voo noturno pra manaus?", None),
        ("gostaria de cotar aereo rio x lisboa", None),
        ("e se eu sair dia 13?", FLIGHTS_SHOWN),
        ("preciso de passagem pro casamento em salvador dia 8", SAVED),
    ],
    "flight-details": [
        ("o 1 tem mala?", FLIGHTS_SHOWN),
        ("qual desses tem bagagem inclusa?", FLIGHTS_SHOWN),
        ("e a da latam, é qual tarifa?", FLIGHTS_SHOWN),
        ("o voo mais caro tem despachada?", FLIGHTS_SHOWN),
    ],
    "search-hotels": [
        ("hotel fazenda em minas no feriado", None),
        ("preciso de estadia em porto alegre de 3 a 6", None),
        ("pousadinha em arraial pro fim de semana", None),
        ("quero ficar num hotel perto do aeroporto de guarulhos hoje", None),
        ("e hotel perto da praia?", HOTELS_SHOWN),
        ("preciso de um lugar pra dormir em recife nessa data", FLIGHTS_SHOWN),
    ],
    "set-preferences": [
        ("sou latam pass platina", None),
        ("nao gosto de voar de madrugada, prefiro sempre de dia", None),
        ("gol eu evito", None),
        ("pode considerar que eu saio sempre de BH", None),
        ("cama ruim é o que mais me irrita em hotel", None),
    ],
    "none": [
        ("oi", None),
        ("qual a franquia de bagagem da gol em geral?", None),
        ("quanto tempo antes devo chegar no aeroporto?", None),
        ("vcs aceitam pix?", None),
        ("blz", FLIGHTS_SHOWN),
        ("o que fazer em recife a noite?", None),
    ],
}


EXTRAS = {
    "flight-extras": [
        ("quanto custa despachar mala no segundo?", FLIGHTS_SHOWN),
        ("tem bilhete flexivel pro voo da azul?", FLIGHTS_SHOWN),
        ("quanto sai o seguro viagem nesse primeiro?", FLIGHTS_SHOWN),
        ("e se eu precisar remarcar o da latam, quanto custa a tarifa flexivel?", FLIGHTS_SHOWN),
        ("qual o preco pra adicionar uma mala de 23kg no da gol?", FLIGHTS_SHOWN),
        ("da pra comprar mala extra no terceiro?", FLIGHTS_SHOWN),
        ("quanto fica o voo 2 com mala despachada?", FLIGHTS_SHOWN),
        ("posso cancelar depois? quanto custa a flexibilidade no primeiro?", FLIGHTS_SHOWN),
    ],
    "flight-details": [
        ("a tarifa do segundo inclui mala?", FLIGHTS_SHOWN),
        ("o da gol ja vem com despachada?", FLIGHTS_SHOWN),
    ],
}

HOLDOUT4 = {
    "flight-extras": [
        ("quanto é pra despachar uma mala no da azul?", FLIGHTS_SHOWN),
        ("o primeiro tem opcao flexivel? quanto?", FLIGHTS_SHOWN),
        ("preciso levar 2 malas no voo da latam, quanto fica?", FLIGHTS_SHOWN),
        ("tem seguro pro segundo voo?", FLIGHTS_SHOWN),
        ("valor da mala extra no mais barato", FLIGHTS_SHOWN),
        ("se eu quiser mudar a data depois no da gol, pago quanto?", FLIGHTS_SHOWN),
    ],
    "flight-details": [
        ("o terceiro inclui bagagem despachada?", FLIGHTS_SHOWN),
        ("qual a tarifa do da azul?", FLIGHTS_SHOWN),
        ("o primeiro é direto?", FLIGHTS_SHOWN),
    ],
    "search-flights": [
        ("quanto custa uma passagem pra salvador com mala?", None),
    ],
    "none": [
        ("quanto custa despachar mala na gol em geral?", None),
        ("seguro viagem é obrigatorio pra europa?", None),
    ],
}


def main():
    n = 0
    cases, prefix = CASES, ""
    if "--holdout" in sys.argv:
        cases, prefix = HOLDOUT, "h-"
    if "--holdout2" in sys.argv:
        cases, prefix = HOLDOUT2, "h2-"
    if "--holdout3" in sys.argv:
        cases, prefix = HOLDOUT3, "h3-"
    if "--v2" in sys.argv:
        cases = {k: CASES.get(k, []) + EXTRAS.get(k, []) for k in list(CASES) + ["flight-extras"]}
    if "--holdout4" in sys.argv:
        cases, prefix = HOLDOUT4, "h4-"
    for want, items in cases.items():
        for i, (message, agent_said) in enumerate(items, 1):
            case = {"id": "%s%s-%02d" % (prefix, want, i), "message": message, "want": want}
            if agent_said:
                case["agentSaid"] = agent_said
                # the Function whose run said it: selector.after reads it
                case["after"] = {FLIGHTS_SHOWN: "search-flights", HOTELS_SHOWN: "search-hotels", SAVED: "set-preferences"}[agent_said]
            print(json.dumps(case, ensure_ascii=False))
            n += 1


if __name__ == "__main__":
    main()
