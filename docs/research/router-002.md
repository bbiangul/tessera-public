# Router 002: the traveler's router on labelled messages

- **Date:** 2026-09-23
- **Question:** with four routable Functions (search-flights,
  flight-details, search-hotels, set-preferences), how often does the
  router start the right one, the wrong one, or none; and is the 0.6
  threshold right? router-001 had six messages; this is the dataset it
  asked for.
- **Setup:** `tessera bench route` sends each message through the host's
  own router (`Host.Route`: the Functions' `selector.description`, the
  agent's last message as `agentSaid`), Jev jev-1.13.0, 3 repeats.
  Datasets are synthetic, written the way people type (no accents,
  abbreviations), some after a list of flights or hotels
  (`bench/datasets/gen_traveler_routing.py`).

| Set | Messages | Role |
| --- | --- | --- |
| `traveler-routing-v1` | 80 | tuning |
| `traveler-routing-holdout-v1` | 34 | written before tuning on v1; later tuning data for a failed attempt |
| `traveler-routing-holdout-v2` | 27 | written before that attempt |
| `traveler-routing-holdout-v3` | 27 | written before that attempt; never tuned on |

Outcomes: *right*; *wrong* (another Function started: the costly error);
*false-route* (an out-of-scope message started one); *unrouted* (an
in-scope message was not routed, so the user is asked again: cheap).

## Results at threshold 0.6 (committed texts)

| Set | right | wrong | false-route | unrouted |
| --- | --- | --- | --- | --- |
| v1 | 98.8% | 0.0% | 0.0% | 1.2% |
| holdout-v1 | 94.1% | 2.9% | 0.0% | 2.9% |
| holdout-v2 | 92.6% | 3.7% | 0.0% | 3.7% |
| holdout-v3 | 100.0% | 0.0% | 0.0% | 0.0% |

Reports per set: `bench/results/008-traveler-routing/`. Before the one
change kept (below), v1 was 97.5% and holdout-v1 94.1%.

## Findings

1. **Follow-ups, flights and out-of-scope messages were never confused**
   (0 errors in 312 trials across the four sets). Context works: "e hotel
   em Recife?" after a list of flights goes to hotels, "e o ultimo?" goes
   to flight-details.
2. **The costly errors are confident.** "flat em sp pra 2 semanas" and
   "chale em campos do jordao" went to search-flights at 0.8–0.94: a place
   and a length of stay read as a trip. No threshold catches them (at 0.9
   they remain). Only better texts or a clarifying step can.
3. **Statements with no request fall to none.** "sou Smiles Diamante",
   "tenho tudoazul", "sou smiles ouro" (0.95 none): the router question
   asks what the message *asks for*.
4. **0.6 is the better threshold on this data.** 0.5 routed "qual a
   franquia de bagagem da gol em geral?" (a general question, out of
   scope) to flight-details at 0.54–0.58 in holdout-v3; 0.6 did not. Above
   0.7 only in-scope messages are lost.
5. **One text change helped; two "principled" ones did not.** Naming
   loyalty programs and "companhias que não quer usar nunca" in
   set-preferences took v1 from 97.5% to 98.8%, holdout-v1 unchanged at
   0.6 (+2 points at 0.5). Then, to fix findings 2 and 3 without words
   from any holdout, the router question became "which capability should
   handle" and the flights and hotels texts excluded each other in general
   terms. Measured: the targeted misses stayed (flat 0.77–0.83 to flights,
   smiles ouro 0.78–0.86 to none) and the clean holdout-v3 got worse (the
   general baggage question rose to 0.69–0.73, past the threshold).
   Reverted.

## Decision

Keep the threshold at 0.6 and the router question as is; keep the
set-preferences text. The confident lodging misroute (finding 2) is open:
the next attempts are a routing text measured against a new holdout, or
routing that sees the inputs a Function extracts. Synthetic sets written
by one author are a floor, not the truth: a set from real anonymized
traffic replaces them when the traveler has users.
`bench/cassettes/traveler-routing-v1.jsonl` replays v1 offline (a test
of the command, not a gate).

## Addendum: the second Decide backend

Same sets and texts, `llm:deepseek/deepseek-v4.1-flash` (llm-constrained,
probabilities from first-token logprobs), 3 repeats
(`bench/results/009-traveler-routing-deepseek/`). Right at 0.6:

| Set | Jev | DeepSeek V4.1 Flash |
| --- | --- | --- |
| v1 (tuning) | 98.8% | 99.6% |
| holdout-v1 | 94.1% | 91.2% |
| holdout-v2 | 92.6% | 82.7% |
| holdout-v3 | 100.0% | 92.6% (3.7% false-route) |
| holdouts together (264 trials) | 95.5% | 89.0% |
| cost per routing decision | $0.000025 | $0.000046 |

- The tuning set does not separate them; the holdouts do. The texts were
  tuned with Jev, which may favor it on v1, not on the holdouts.
- DeepSeek's errors are certain: "tem algo com cancelamento gratis?"
  after a list of hotels went to flight-details at 1.00, "sou smiles
  ouro" to none at 1.00. A threshold cannot catch them; Jev's misses sat
  nearer the threshold.
- Both pass the traveler EvalSuite: 18/18 in 3 live runs each, which is
  the phase 1 exit criterion (RFC-0001, "Migração e roadmap"). The suite
  has 18 routing decisions; the holdouts have 264: conversations prove the
  flows, datasets measure the router.

Consistent with ADR-0003: Jev stays the Decide engine; llm-constrained is
the fallback backend, and it works.

## Addendum 2: a fifth Function, guarded follow-ups, and the threshold

`flight-extras` (checked-bag prices, flexible ticket, insurance for a
flight already shown) was added with a tuning set (`v2` = v1 + extras) and
a holdout written with it (`holdout-v4`), before any text was tuned
(`bench/results/011-traveler-routing-follow-ups/`).

1. **Extras vs details was not the problem** (holdout-v4: 100% of both).
   **General questions were:** "quanto custa um seguro viagem?" and
   "despachar mala na gol em geral?", with no flight shown, went to
   flight-extras at 0.94–0.98. Wording ("sem voos mostrados... não é isto")
   only lowered it to 0.60–0.67, still routed, and adding the fifth
   Function pushed a general baggage question in holdout-v3 to
   flight-details (0.65).
2. **A deterministic guard fixed it:** `selector.after` makes a Function
   an option only when the conversation's last run was one of the named
   Functions. With it, false routes are 0 on all six sets; holdout-v3 and
   v4 are 100%.
3. **Threshold, again from data:** with follow-ups guarded, 0.5 was equal
   or better than 0.6 on every set (holdout-v1 94.1% vs 91.2%, holdout-v2
   92.6% vs 90.1%) with no added wrong or out-of-scope route; the 0.5
   false route that made 0.6 the default came from a follow-up, now
   impossible without a search. The threshold is per namespace
   (`Agent.spec.routing.minConfidence`): traveler 0.5; the default stays 0.6.
4. **Rejected, measured:** asking the flight pick as "which option is the
   message about?" instead of "which option does it choose?" scored 92.1%
   vs 97.4% on the follow-up picks. Kept "choose". Ordinal questions about
   prices sit near the 0.7 bar ("quanto custa despachar mala no segundo?"
   0.69–0.72, "...no primeiro?" ~0.5): the agent then asks "Sobre qual
   voo?", which is safe; tracked in `traveler-followups-extras-v1.jsonl`.
5. "Passagem de São Paulo pra Recife ontem" (an EvalSuite case, not a
   holdout) fell to 0.50 against none with five options; the flights text
   now says a wrong or past date is still a search. 5/5 routed after, no
   set moved.
