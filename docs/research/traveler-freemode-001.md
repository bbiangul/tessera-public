# Traveler free mode 001: the free mode on a second domain

- **Date:** 2026-09-25.
- **Pre-registration:** `traveler-freemode-001-preregistration.md`.
- **Data:** `bench/results/traveler-freemode-001/`.
- **Spend:** ~$0.22 in total, of which $0.21 was the judge.

## Result

| | baseline (today's traveler) | freemode (+ `freeMode.instructions`) |
| --- | --- | --- |
| Dead ends (of 48) | **48 (100%)** | **14 (29%)** |
| Helpful | 0 | 34 (71%) |
| False claims | 0 | **0** |
| Humane (1–5) | 1.88 | 3.69 |
| Started a search | 10 | 11 |

**Hypotheses**

| # | Result | Detail |
| --- | --- | --- |
| T1 | **Supported** | Every off-script message dead-ended in the baseline. Some got "Desculpe, não entendi"; the rest were routed to a search that asked for a date. |
| T2 | **Not supported** (≤ 10% registered, 29% measured) | 11 of the 14 dead ends never reached the free mode (the router's, see below). The other 3 are `best-season`, a redirection by design. |
| T3 | **Supported** | 0 false claims in 48. Every question about current data (baggage, visas, pets, insurance, cancelling elsewhere) was answered "I don't have that here" plus an offer. |
| T4 | **Supported** | Helpful went from 0% to 71% (+71 points). |
| T5 | **Supported for searches** | Every flight, hotel and profile scenario passes live: 39/39, against 40/40 in the baseline. The one failure (`profile/out-of-scope`) asserts the old text, "Desculpe, não entendi", which the free mode replaces as intended. Its `never` (no search started) passed. |

## Where the dead ends come from

- **Router, not the free mode (11/14):**
  - `avg-price` ("quanto custa em média uma diária em Gramado?"), 3/3;
  - `hotel-vs-flat`, 3/3;
  - `neighborhood` ("qual bairro é melhor pra ficar em Recife?"), 3/3;
  - `flight-time`, 2/3.

  These are questions about a destination, and the router sends them to
  search-hotels or search-flights. The search then asks "Qual o dia do
  check-in?" with no word on why. This is the misrouting router-003
  measured, and it is identical in both variants: routing is shared, and
  the free mode only sees unrouted messages.
- **Free mode by design (3/14):** `best-season` ("qual a melhor época pra
  ir pra Jericoacoara?"). The free mode answers only from facts, and the
  traveler has none, so it declined and offered a search. The judge wanted
  general knowledge ("dry season is …"). The platform's rule (no facts, no
  claims) is what kept false claims at 0. Giving general advice is a
  product decision, not a fix: it would need a read with that knowledge,
  or an explicit allowance.

## What this means

1. **The free mode generalises.** A second domain, one paragraph of
   instructions, no reads: dead ends drop from 100% to 29%, helpfulness
   rises from 0% to 71%, and nothing is invented.
2. **What remains is routing.** A general question about a destination
   looks like a search. Two candidate fixes, to be measured on new
   messages and not on this set:
   - selector descriptions that say what the search is not ("not for
     general questions about a destination: prices in general, which
     neighbourhood, how long a flight takes");
   - a search's first question that says why it asks ("Para ver os preços
     em Gramado, qual o dia do check-in?").
3. **For the authoring skill:** an agent without reads still gets a free
   mode. Its instructions say what it does, what it does not, and what to
   offer instead.

**Not changed:**
- `examples/traveler` (the baseline copy is the example as of c0f02de);
- the `profile/out-of-scope` scenario.

Adopting the free mode in the example would change that scenario's
assertion. That decision is left for the #36 work on authored-agent
quality.
