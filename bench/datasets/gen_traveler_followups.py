#!/usr/bin/env python3
"""Generates traveler-followups-v1.jsonl: which previously shown flight a
follow-up question refers to ("e a bagagem do segundo?"). The question is
exactly the one the runtime asks Decide for a choice ask (options o1..oN
labeled as the agent showed them, plus none). Gold "none" means the message
does not pick one flight, and the agent must ask back.
Run: python3 bench/datasets/gen_traveler_followups.py"""
import json
import os

A = [
    "1. GOL GRU → REC, 12/10 12:40–15:55 (direto, 3h15) — R$ 640",
    "2. Azul GRU → REC, 12/10 16:05–19:30 (direto, 3h25) — R$ 720, mala despachada inclusa",
    "3. LATAM GRU → REC, 12/10 07:10–10:25 (direto, 3h15) — R$ 890",
]
B = [
    "1. LATAM GRU → LIS, 02/10 06:00–12:30 (1 parada, 6h30) — R$ 410",
    "2. GOL GRU → LIS, 02/10 12:40–15:55 (direto, 3h15) — R$ 640",
    "3. Azul GRU → LIS, 02/10 21:30–08:15 (1 parada, 10h45) — R$ 505",
    "4. LATAM GRU → LIS, 02/10 23:55–03:10 (direto, 3h15) — R$ 560",
]

CASES = [
    ("second", A, "e a bagagem do segundo?", "o2"),
    ("first", A, "qual a bagagem do primeiro?", "o1"),
    ("third", A, "o terceiro inclui mala?", "o3"),
    ("airline-gol", A, "e o da GOL?", "o1"),
    ("airline-azul", A, "o da Azul tem mala despachada?", "o2"),
    ("airline-latam", A, "o voo da LATAM permite remarcar?", "o3"),
    ("cheapest", A, "o mais barato inclui bagagem?", "o1"),
    ("most-expensive", A, "e o mais caro?", "o3"),
    ("morning", A, "o de manhã", "o3"),
    ("at-16", A, "o das 16h", "o2"),
    ("last", A, "o último", "o3"),
    ("this-first", A, "esse primeiro aí", "o1"),
    ("no-reference", A, "e a bagagem?", "none"),
    ("gol-noon", A, "o da Gol que sai meio dia", "o1"),
    ("option-2", A, "a opção 2", "o2"),
    ("english", A, "number 3 please", "o3"),
    ("late-afternoon", A, "o de fim de tarde", "o2"),
    ("arrives-earliest", A, "o que chega mais cedo", "o3"),
    ("by-price", A, "o de R$ 720", "o2"),
    ("fare-at-7", A, "quero saber da tarifa do voo das 7", "o3"),
    ("lowercase-azul", A, "o da azul", "o2"),
    ("just-2", A, "o 2", "o2"),
    ("at-1240", A, "bagagem do voo das 12:40", "o1"),
    ("direct-night", B, "o direto da noite", "o4"),
    ("stop-morning", B, "o com escala de manhã", "o1"),
    ("b-azul", B, "o da Azul", "o3"),
    ("cheapest-direct", B, "o direto mais barato", "o4"),
    ("both-latam", B, "e os dois da LATAM?", "none"),
    ("at-2130", B, "o que sai 21h30", "o3"),
    ("fourth", B, "o quarto", "o4"),
    ("by-price-410", B, "o de 410 reais", "o1"),
    ("gol-nonstop", B, "o sem escala da GOL", "o2"),
]

QUESTION = "Sobre qual voo?"

if __name__ == "__main__":
    here = os.path.dirname(os.path.abspath(__file__))
    with open(os.path.join(here, "traveler-followups-v1.jsonl"), "w", encoding="utf-8") as f:
        for cid, flights, message, gold in CASES:
            options = [{"key": f"o{i+1}", "description": label} for i, label in enumerate(flights)]
            options.append({"key": "none", "description": "none of the options, or not clear"})
            f.write(json.dumps({
                "id": cid, "task": "followup-choice", "state": {"message": message},
                "question": {"kind": "choice",
                             "instructions": "Which option does the user's message choose? The question the user was asked: " + QUESTION,
                             "options": options},
                "gold": gold}, ensure_ascii=False) + "\n")
    print(len(CASES), "cases")
