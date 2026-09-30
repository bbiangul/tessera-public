# router-004: general travel questions no longer start a search

- **Date:** 2026-09-25.
- **Pre-registration:** `router-004-preregistration.md`.
- **Issue:** #48.
- **Data:** `bench/results/router-004/`.
- **Setup:** Jev 1.13.0, 3 repeats, threshold 0.5.
- **Spend:** under $0.05.

## The change

The two searches' `selector.description` now:
- say they are a request for the agent to **look for** options;
- list, as examples, the opinion and information questions they are not
  for.

**Hotels:** "Não inclui perguntas de opinião ou comparação sobre um
destino, que não pedem que eu procure (pousada ou hotel?, hotel ou flat?,
qual bairro é melhor?, o lugar é caro?)".

**Flights:** "…nem perguntas de informação ou comparação sobre uma
viagem, que não pedem que eu procure voos (quanto tempo dura o voo?,
costuma ter escala?, qual companhia voa para lá?, qual aeroporto é
melhor?)".

**Development:**
- the first wording, "general questions about a destination", left
  "pousada ou hotel em Paraty?" and "Gramado é caro?" routed (5/21);
- phrasing it by the verb, a request to look for options against a
  question of opinion or comparison, brought that to 0/21, with 15/15
  searches kept.

## Result

| | A (before) | B (after) |
| --- | --- | --- |
| **Test** (sealed, 18 messages × 3) | | |
| General questions sent to a search (30) | 9 (30%) | **0** |
| Real searches routed right (24) | 24 | 24 |
| **Regression** (318 used messages × 3) | | |
| Right | 96.1% | 96.8% |
| Wrong (a wrong Function, or an out-of-scope message routed) | 2.6% | 1.6% |
| Unrouted in-scope messages | 12 | 16 |

| # | Hypothesis | Result |
| --- | --- | --- |
| R1 | Test `none` routed ≤ 10% | **Supported** (0/30; A 9/30) |
| R2 | Test searches ≥ 90% and ≥ A − 1 | **Supported** (24/24) |
| R3 | Regression right ≥ A − 1 point, wrong ≤ A + 1 point | **Supported** (+0.7, −1.0) |

**A's test misses** were "quanto tempo de voo de Recife pra Lisboa?",
"hotel ou flat… em São Paulo?" and "qual companhia aérea voa direto pra
Lisboa?", all 3 of 3.

**What B lost:** one follow-up, "tem algo com cancelamento gratis?" after
a hotel list (holdout-v1, 3 of 3), is no longer routed to a new search.
It reaches the free mode, which sees the list in the conversation. B
gained three general questions in the regression sets ("hotel ou flat",
"melhor bairro em Recife") and "flat em sp pra 2 semanas".

**Applied** to `examples/traveler`; its cassette was re-recorded (24/24
live and on replay).
