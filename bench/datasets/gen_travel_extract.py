#!/usr/bin/env python3
"""Generates travel-extract-v1.jsonl: synthetic pt-BR flight requests with
gold extractions. Dates are computed here from TODAY, never typed by hand.
A gold value {"$any": [...]} accepts any of the listed values (ambiguous
phrasing). Run: python3 bench/datasets/gen_travel_extract.py"""
import datetime as dt
import json
import os

TODAY = dt.date(2026, 9, 23)  # a Wednesday


def d(days=None, y=None, m=None, day=None):
    if days is not None:
        return (TODAY + dt.timedelta(days=days)).isoformat()
    return dt.date(y, m, day).isoformat()


def case(cid, msg, dest=None, origin=None, depart=None, ret=None, adults=1, children=(), cabin="ECONOMY"):
    return {"id": cid, "message": msg, "gold": {
        "destination": dest, "origin": origin, "departDate": depart, "returnDate": ret,
        "adults": adults, "childrenAges": list(children), "cabinClass": cabin}}


ANY = lambda *v: {"$any": list(v)}

CASES = [
    case("explicit-date", "Quero ir para Lisboa dia 12 de outubro", "Lisboa", depart=d(y=2026, m=10, day=12)),
    case("origin-tomorrow", "Preciso de passagem de São Paulo para Recife amanhã", "Recife", "São Paulo", d(1)),
    case("day-after-sunday", "voo pra Salvador depois de amanhã, volta domingo", "Salvador", depart=d(2), ret=d(4)),
    case("range-spouse", "Quero ir a Paris de 10 a 20 de novembro com minha esposa", "Paris",
         depart=d(y=2026, m=11, day=10), ret=d(y=2026, m=11, day=20), adults=2),
    case("family-kids", "Passagem para Orlando em dezembro, dia 15, eu, meu marido e nossos filhos de 5 e 10 anos, volta dia 28",
         "Orlando", depart=d(y=2026, m=12, day=15), ret=d(y=2026, m=12, day=28), adults=2, children=(5, 10)),
    case("next-monday-infant", "Buenos Aires semana que vem, segunda-feira, com meu bebê de 8 meses", "Buenos Aires",
         depart=d(5), children=(0,)),
    case("business-next-year", "Executiva para Tóquio em 3 de março, só ida", ANY("Tóquio", "Tokyo"),
         depart=d(y=2027, m=3, day=3), cabin="BUSINESS"),
    case("nothing", "quero viajar", None),
    case("in-15-days", "Rio de Janeiro daqui a 15 dias, volto 4 dias depois", "Rio de Janeiro", depart=d(15), ret=d(19)),
    case("friday-next-friday", "Vou pra Miami na sexta e volto na outra sexta", "Miami", depart=d(2), ret=d(9)),
    case("three-adults-numeric", "3 adultos para Cancún, ida 5/11 volta 12/11", "Cancún",
         depart=d(y=2026, m=11, day=5), ret=d(y=2026, m=11, day=12), adults=3),
    case("abbrev-origin", "de BH para Porto Alegre no dia 30", "Porto Alegre", ANY("BH", "Belo Horizonte"), d(7)),
    case("first-class", "Primeira classe para Dubai, 1º de dezembro", "Dubai", depart=d(y=2026, m=12, day=1), cabin="FIRST"),
    case("tomorrow-morning", "Quero ir a Santiago amanhã de manhã", "Santiago", depart=d(1)),
    case("premium-economy", "Premium economy para Londres, 20 de janeiro, voltando 2 de fevereiro", "Londres",
         depart=d(y=2027, m=1, day=20), ret=d(y=2027, m=2, day=2), cabin="PREMIUM_ECONOMY"),
    case("stay-a-week", "Florianópolis no feriado de 12 de outubro, fico uma semana", "Florianópolis",
         depart=d(y=2026, m=10, day=12), ret=d(y=2026, m=10, day=19)),
    case("friends-ten-days", "eu e mais dois amigos, Nova York, 10 dias a partir de 1/10", "Nova York",
         depart=d(y=2026, m=10, day=1), ret=ANY(d(y=2026, m=10, day=11), d(y=2026, m=10, day=10)), adults=3),
    case("origin-after-dest", "Lisboa, saio de Porto Alegre em 7 de novembro", "Lisboa", "Porto Alegre", d(y=2026, m=11, day=7)),
    case("tonight", "passagem pra Manaus hoje à noite", "Manaus", depart=d(0)),
    case("two-small-kids", "Roma, com minha filha de 2 anos e meu filho de 1 ano, 18 de dezembro", "Roma",
         depart=d(y=2026, m=12, day=18), children=(2, 1)),
    case("month-only", "Madri em outubro", "Madri"),
    case("next-saturday", "Quero voar para Brasília no próximo sábado e voltar no domingo", "Brasília", depart=d(3), ret=d(4)),
    case("new-year", "Com 2 crianças de 3 e 7 anos e 2 adultos para Porto Seguro, 20/12 a 3/1", "Porto Seguro",
         depart=d(y=2026, m=12, day=20), ret=d(y=2027, m=1, day=3), adults=2, children=(3, 7)),
    case("business-mother", "Fortaleza, ida em 15 de outubro, volta 22 de outubro, executiva, eu e minha mãe", "Fortaleza",
         depart=d(y=2026, m=10, day=15), ret=d(y=2026, m=10, day=22), adults=2, cabin="BUSINESS"),
    case("no-date", "Não sei ainda a data, mas quero ir para Natal", "Natal"),
    case("next-tuesday", "Viagem de negócios para Curitiba na terça que vem, volto quarta", "Curitiba", depart=d(6), ret=d(7)),
    case("lap-infant", "Seul, 1 adulto e 1 bebê de colo, 14 de fevereiro", ANY("Seul", "Seoul"),
         depart=d(y=2027, m=2, day=14), children=(0,)),
    case("day-only", "de Campinas pra Belém dia 2", "Belém", "Campinas", d(y=2026, m=10, day=2)),
    case("round-trip-range", "ida e volta para Montevidéu, 6 a 9 de novembro", "Montevidéu",
         depart=d(y=2026, m=11, day=6), ret=d(y=2026, m=11, day=9)),
    case("weekend", "Quero ir pra Maceió no fim de semana", "Maceió", depart=ANY(d(2), d(3), None), ret=ANY(d(4), None)),
]

INSTRUCTIONS = (
    "Extract the flight search from the user's message. STATE.today is today's date and weekday. "
    "Dates are YYYY-MM-DD: resolve relative dates (amanhã, sexta, semana que vem) against today; a date "
    "without a year is its next occurrence from today; a return given as a duration counts from the "
    "departure. Use null when the message has no specific day (only a month, or nothing). "
    "adults counts the adults traveling, including the user (default 1). childrenAges lists the ages of "
    "children under 18; a baby under 1 year is 0. cabinClass is ECONOMY unless the message asks for "
    "another. Keep place names as the user wrote them; null when absent.")

SCHEMA = {
    "type": "object", "additionalProperties": False,
    "required": ["destination", "origin", "departDate", "returnDate", "adults", "childrenAges", "cabinClass"],
    "properties": {
        "destination": {"type": ["string", "null"]},
        "origin": {"type": ["string", "null"]},
        "departDate": {"type": ["string", "null"], "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
        "returnDate": {"type": ["string", "null"], "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
        "adults": {"type": "integer", "minimum": 1, "maximum": 9},
        "childrenAges": {"type": "array", "items": {"type": "integer", "minimum": 0, "maximum": 17}},
        "cabinClass": {"enum": ["ECONOMY", "PREMIUM_ECONOMY", "BUSINESS", "FIRST"]},
    },
}

WEEKDAYS = ["segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado", "domingo"]

if __name__ == "__main__":
    here = os.path.dirname(os.path.abspath(__file__))
    today = f"{TODAY.isoformat()} ({WEEKDAYS[TODAY.weekday()]})"
    with open(os.path.join(here, "travel-extract-v1.task.json"), "w", encoding="utf-8") as f:
        json.dump({"instructions": INSTRUCTIONS, "schema": SCHEMA}, f, ensure_ascii=False, indent=2)
        f.write("\n")
    with open(os.path.join(here, "travel-extract-v1.jsonl"), "w", encoding="utf-8") as f:
        for c in CASES:
            c = dict(c, state={"message": c.pop("message"), "today": today})
            f.write(json.dumps(c, ensure_ascii=False) + "\n")
    print(f"{len(CASES)} cases")
