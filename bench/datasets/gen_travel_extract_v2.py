#!/usr/bin/env python3
"""Generates travel-extract-v2.jsonl: harder synthetic pt-BR flight requests
(typos, slang, corrections, holidays by name, numbers in words, English
mixed in, negations). Same task file and fields as v1; dates computed here
from TODAY. {"$any": [...]} accepts any listed value.
Run: python3 bench/datasets/gen_travel_extract_v2.py"""
import datetime as dt
import json
import os

from gen_travel_extract import TODAY, WEEKDAYS, ANY, case, d

CASES = [
    case("typos", "qro ir pra lisbooa dia 12 d outubro", ANY("Lisboa", "lisbooa"), depart=d(y=2026, m=10, day=12)),
    case("correction", "voo pra Recife dia 10, não, dia 12 de outubro", "Recife", depart=d(y=2026, m=10, day=12)),
    case("correction-destination", "quero ir pra Salvador... na verdade Fortaleza, dia 5 de novembro", "Fortaleza",
         depart=d(y=2026, m=11, day=5)),
    case("christmas", "passagem pra Porto Alegre pro Natal", "Porto Alegre", depart=ANY(d(y=2026, m=12, day=25), d(y=2026, m=12, day=24), None)),
    case("new-years-eve", "quero passar o réveillon em Florianópolis, vou dia 29 e volto dia 2", "Florianópolis",
         depart=d(y=2026, m=12, day=29), ret=d(y=2027, m=1, day=2)),
    case("words-numbers", "dois adultos e três crianças de 4, 6 e 9 anos para Orlando, 3 de janeiro", "Orlando",
         depart=d(y=2027, m=1, day=3), adults=2, children=(4, 6, 9)),
    case("english", "flight to NYC on Oct 5th for 2 people", ANY("NYC", "New York", "Nova York"), depart=d(y=2026, m=10, day=5), adults=2),
    case("cabin-slang", "classe executiva pra Paris dia 20 de novembro", "Paris", depart=d(y=2026, m=11, day=20), cabin="BUSINESS"),
    case("negated-cabin", "Madri dia 8 de dezembro, não precisa ser executiva", "Madri", depart=d(y=2026, m=12, day=8)),
    case("nickname-origin", "saindo de Floripa pra BH dia 15 de outubro", ANY("BH", "Belo Horizonte"),
         ANY("Floripa", "Florianópolis"), d(y=2026, m=10, day=15)),
    case("teen-child", "eu e meu filho de 17 anos, Cancún, 14 de novembro", "Cancún", depart=d(y=2026, m=11, day=14), children=(17,)),
    case("in-laws", "eu, minha esposa e minha sogra pra Natal dia 3 de dezembro", "Natal", depart=d(y=2026, m=12, day=3), adults=3),
    case("baby-and-child", "com um bebê de 2 meses e uma criança de 3 anos, Recife, 1 de novembro", "Recife",
         depart=d(y=2026, m=11, day=1), children=(0, 3)),
    case("return-in-days", "Santiago dia 18 de outubro voltando 3 dias depois", "Santiago",
         depart=d(y=2026, m=10, day=18), ret=d(y=2026, m=10, day=21)),
    case("next-week-weekday", "na semana que vem, na terça, pra Curitiba", "Curitiba", depart=d(6)),
    case("this-friday", "essa sexta pra Vitória", ANY("Vitória", "Vitoria"), depart=d(2)),
    case("month-and-week", "Roma na primeira semana de dezembro", "Roma", depart=ANY(None, d(y=2026, m=12, day=1))),
    # "voltar ... dia 30" reads as a return date or as the day of a one-way
    # flight home; both readings are accepted (the first run of v2 showed it).
    case("only-return", "quero voltar de Lisboa dia 30", None, origin="Lisboa", depart=ANY(None, d(7)), ret=ANY(None, d(7))),
    case("no-destination", "tem voo barato amanhã?", None, depart=d(1)),
    case("question-form", "quanto custa uma passagem pra Manaus em 10 de novembro?", "Manaus", depart=d(y=2026, m=11, day=10)),
    case("many-details", "Sou de Campinas, quero ir pra Buenos Aires com minha esposa e nossa filha de 5 anos, ida 12/12, volta 19/12, econômica",
         "Buenos Aires", "Campinas", d(y=2026, m=12, day=12), d(y=2026, m=12, day=19), adults=2, children=(5,)),
    case("first-class-words", "primeira classe pra Dubai dia primeiro de março", "Dubai", depart=d(y=2027, m=3, day=1), cabin="FIRST"),
    case("premium-words", "premium economy pra Londres, 7 de fevereiro", "Londres", depart=d(y=2027, m=2, day=7), cabin="PREMIUM_ECONOMY"),
    case("day-after-tomorrow-slang", "depois d amanhã pra Goiânia", "Goiânia", depart=d(2)),
    case("in-two-weeks", "daqui a duas semanas pra Belém", "Belém", depart=d(14)),
    case("round-trip-weekend", "sexta que vem vou pra João Pessoa e volto domingo", "João Pessoa",
         depart=ANY(d(2), d(9)), ret=ANY(d(4), d(11))),
    case("group", "somos 5 adultos, Porto Seguro, 2 de janeiro", "Porto Seguro", depart=d(y=2027, m=1, day=2), adults=5),
    case("hotel-not-flight-dates", "vou pra Gramado dia 20 de dezembro e fico até o dia 27", "Gramado",
         depart=d(y=2026, m=12, day=20), ret=d(y=2026, m=12, day=27)),
    case("pet-noise", "levo meu cachorro, voo pra Aracaju dia 9 de outubro", "Aracaju", depart=d(y=2026, m=10, day=9)),
    case("past-date", "passagem pra Maceió dia 10 de setembro", "Maceió", depart=ANY(d(y=2027, m=9, day=10), d(y=2026, m=9, day=10))),
]

if __name__ == "__main__":
    here = os.path.dirname(os.path.abspath(__file__))
    today = f"{TODAY.isoformat()} ({WEEKDAYS[TODAY.weekday()]})"
    with open(os.path.join(here, "travel-extract-v2.jsonl"), "w", encoding="utf-8") as f:
        for c in CASES:
            c = dict(c, state={"message": c.pop("message"), "today": today})
            f.write(json.dumps(c, ensure_ascii=False) + "\n")
    print(f"{len(CASES)} cases")
