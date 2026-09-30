# Router 001: what the router reads matters more than the model

- **Date:** 2026-09-23
- **Question:** the traveler's first live EvalSuite run sent "quero viajar"
  and "Passagem de São Paulo pra Recife ontem" to *none*. Is Jev failing,
  or is the routing text?
- **Setup:** one Decide choice question, "Which capability does the user's
  message ask for?", with two options (the Function and *none*), Jev
  jev-1.13.0. Only the Function's description changes.

| Message | English description (the Function's own) | Portuguese routing text |
| --- | --- | --- |
| quero viajar | none, P(flights) = 0.09 | flights, 0.91 |
| Passagem de São Paulo pra Recife ontem | flights, 0.62 | flights, 0.84 |
| Quero voar de São Paulo para Recife dia 12 de outubro | flights, 1.00 | flights, 1.00 |
| tem voo pra Lisboa amanhã? | flights, 0.99 | flights, 1.00 |
| bom dia! | none, 0.00 | none, 0.00 |
| qual o horário da farmácia? | none, 0.00 | none, 0.00 |

English: "Searches flights between two cities on a date and recommends the
best ones." Portuguese: "Busca passagens aéreas e voos: o usuário quer
viajar, voar ou comprar passagem para algum lugar, com ou sem datas."

## Findings

1. **The routing text, not the model, decided the failures.** Written in
   the users' language and describing how they ask ("quer viajar, voar ou
   comprar passagem"), it routed the vague request with 0.91 and kept
   small talk at 0.00.
2. **A text for people and a text for the router are different jobs.**
   `spec.description` explains the Function to authors; routing needs the
   users' words.

## Decision

`selector.description` is the router's text (falling back to
`spec.description`). The guidance for authors: write it in the users'
language, with the words they use. Six messages are an anecdote, not a
benchmark: a routing dataset per tenant (from real, anonymized traffic)
is the next measurement, and the threshold (0.6) is set from it.

## Addendum: context (same day)

A follow-up like "e o da LATAM?" names no capability by itself. With only
the message, Jev routed it to *none*; with the agent's previous message in
the state (`agentSaid`, masked), it routed to `flight-details` and the
EvalSuite scenario `follow-up-baggage` passes. One case; the routing
dataset planned above must include follow-ups with and without context.

## Addendum 2: routing text between two Functions (same day)

With `flight-details` added, "e o da LATAM?" (after the agent listed
flights) failed 2 of 4 live runs. Measured with 5 calls each:

| Message | P(flight-details), first texts | P(flight-details), sharper texts |
| --- | --- | --- |
| e o da LATAM? | 0.30–0.38 (routed to search) | 0.83–0.90 |
| e a bagagem do segundo? | 1.00 | 1.00 |
| e pra Salvador? (a new search) | 0.04–0.05 | 0.03–0.04 |
| quero ir pra Lisboa agora | 0.00 | 0.00 |

The sharper texts say what each Function is *not* ("uma busca NOVA … não
inclui perguntas sobre voos já mostrados") and give examples of the short
follow-ups. Jev also varied by about ±0.04 between identical calls, which
is why a route near the threshold flips; EvalSuites replay recorded
answers, and live runs should be repeated before a route is trusted.
