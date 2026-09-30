# websearch-002: not searching for action requests; not concluding on the user's own case. Pre-registration

- **Date:** 2026-09-25, before any change.
- **Issue:** #50.
- **Two failures from websearch-001:**
  - **W5:** "aluga um carro pra mim em Lisboa" was searched 3 of 3 times,
    on both providers.
  - **The passport case** (TinyFish addendum): the answer applied a web
    rule to the user's own dates wrongly ("provavelmente atende"), 3 of 3
    times.

## Changes (built after this is registered, tuned only on `dev`)

1. **The search question:** a request for the agent to do something
   (book, buy, rent, cancel, check in) is not a search, even when the
   agent cannot do it.
2. **A web rule:** do not decide whether the user's own case meets a rule
   found on the web (their dates, documents, age, weight, eligibility).
   State the rule, its source and what to check; the company or the
   authority confirms.

## Sets (written and hashed now)

**`bench/websearch/messages-002.json`** (sha256 prefix `d410d01bbe0a9a08`):
- `dev`: 10 messages;
- `test`: 16, sealed:
  - 4 `action` (van, check-in, insurance, buggy tour);
  - 4 `owncase` (the passport's dates, age 16, an 11 kg bag, a
    15-year-old vaccine);
  - 6 `current`;
  - 2 `nosearch`.

## Variants and runs

| | |
| --- | --- |
| **B** | Today's code (8855d62) |
| **C** | B with the two changes |
| **Setup** | Both use `bench/websearch/traveler-web`, provider **TinyFish**, 3 repeats, runs one at a time (the free tier's rate limit) |
| **Judge** | websearch-001's, with one more field, `concludes_own_case`: the reply says whether the user's own case meets the rule |

## Hypotheses (on `test`, C unless said)

| # | Hypothesis |
| --- | --- |
| X1 | `action` messages search in ≤ 1 of 12 runs, and every one still declines and says what the agent does |
| X2 | `owncase`: false claims ≤ 1 of 12, and `concludes_own_case` in ≤ 2 of 12 |
| X3 | No regression on `current` (18 runs): answered ≥ 80%, false ≤ 2, cites ≥ 90%. Also ≥ B − 2 answered |
| X4 | `nosearch` searches ≤ 1 of 6. The traveler EvalSuites still pass live with C (24/24) |

**Budget:** $2.
