# websearch-002: no searches for action requests; fewer conclusions on the user's own case

- **Date:** 2026-09-25.
- **Pre-registration:** `websearch-002-preregistration.md`.
- **Issue:** #50.
- **Data:** `bench/results/websearch-002/`.
- **Setup:** provider TinyFish, 3 repeats, runs one at a time (0 provider
  errors).
- **Spend:** ~$0.35, almost all the judge.

## The changes (tuned on `dev` only)

1. **The search question** says a request for the agent to do something
   (book, buy, rent, cancel, check in, send) is not a search, even when
   the agent cannot do it.
2. **A web rule:** do not decide whether the user's own case meets a
   rule found on the web. State the rule, its source and what to check;
   never "yes, you can" or "no, you cannot" for their case.

## Result (sealed `test`)

| | B (before) | C (after) |
| --- | --- | --- |
| `action` runs that searched (12) | 6 | **0** |
| `action` handled well (12) | 12 | 12 |
| `owncase`: concludes on the user's case (12) | 9 | **3** |
| `owncase`: false claims (12) | 0 | 1 |
| `current` answered (15 judged) | 15 | 15 |
| `current` false claims | 0 | 0 |
| `current` cites | 15/15 | 15/15 |
| `nosearch` runs that searched (6) | 0 | 0 |
| Traveler EvalSuites, live | — | 24/24 |

| # | Hypothesis | Result |
| --- | --- | --- |
| X1 | `action` searches ≤ 1/12, and they still decline well | **Supported** (0/12; 12/12) |
| X2 | `owncase` false ≤ 1/12 and concludes ≤ 2/12 | **Not supported** (false 1, concludes **3**) |
| X3 | No regression on `current` | **Supported** (15/15 answered, 0 false, 100% cited) |
| X4 | `nosearch` ≤ 1/6; suites pass | **Supported** (0/6; 24/24) |

- **The registered 18 `current` runs:** `t2-cur-rec-lis` ("quanto tempo
  de voo de Recife pra Lisboa?") was routed to the flight search in both
  variants, 3 of 3 (#48), so 15 runs reached the free mode.
- **X2, the three conclusions:** all were `t2-own-vaccine` ("tomei a
  vacina de febre amarela há 15 anos, ainda vale?"). The gov.br rule
  says one dose is valid for life. Saying it still counts applies the
  rule, and here the rule settles the case without reading the user's
  dates.

  The dangerous kind, a date calculation (`t2-own-passport`) or a weight
  over a limit (`t2-own-11kg`), no longer concludes: 0 of 6, against
  B's passport and bag conclusions. Registered is registered: X2 fails.
- **The one false claim** (`t2-own-11kg`): a 10 kg limit taken from an
  Instagram post was attributed to the airline's site.
- **Not measured here:** multi-turn follow-ups ("e para crianças?" after
  a visa answer), which stay open in #50.
