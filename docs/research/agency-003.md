# agency-003: relayed replies, the rest of a reply, booking status

- **Date:** 2026-09-28.
- **Pre-registration:** `agency-003-preregistration.md` (commit 2659a49).
- **Data:** `bench/results/agency-003/`.
- **Variants:**
  - **C:** `bench/agency/c2`, the traveler and platform of be2a4d7;
  - **D:** 1a5dbe3 and c625c45. c625c45 only moves tests.
- **Spend:** about $3.5.

## Result (sealed test: 12 new personas × 3)

| | C | D |
| --- | --- | --- |
| Goal met (of 36) | 21 (58%) | 21 (58%) |
| Redundant messages | 41 | 35 |
| Natural (1–5) | 3.03 | 3.25 |
| Trust (of 36) | 15 | 19 |
| Information lost | 32 | 36 |
| Conversations with a false statement | 13 | 13 |
| Invariant failures | 0 | 0 |
| Median agent cost | $0.0031 | $0.0025 |

| # | Hypothesis | Result |
| --- | --- | --- |
| M1 | Goal ≥ 65% and ≥ C + 15 points | **Not supported:** 58% and 58% |
| M2 | Redundant ≤ 60% of C's | **Not supported:** 85% |
| M3 | Natural ≥ C + 0.4 | **Not supported:** +0.22 |
| M4 | Conversations with a false statement ≤ C's | **Supported:** 13 and 13 |
| M5 | Invariants; median cost ≤ $0.02 | **Supported** |
| M6 | Regressions the same day | **Supported:** routing right 95.1% vs 95.8%, wrong 1.5% vs 1.5% (out of scope 1.5% vs 0.9%); fallback-001 set controls 36/36, overreach 4/27 both; suites 35/35 live; `make ci`, `make lint-extra` |

**Per persona, D:**
- **Solved every time, where the build aimed:**
  - replying to the team with a question, then confirming (v02: 3/3,
    natural 3.7);
  - data that comes tomorrow (v03: 3/3, 4.0);
  - booking status (v04: 3/3, 5.0);
  - cancelling with a negotiation through the team (v05: 3/3, 4.3);
  - context given before the request (v11: 3/3, 5.0);
  - a baggage rule and price (v12: 3/3, 5.0).
- **Failed every time:**
  - a yes with a question (v01: 0/3);
  - two questions at once with a quote (v06);
  - "resend my e-mail" (v08: 0/3, lost 11);
  - insurance from the team (v10);
- **Mostly failed:** "the same a week later" (v09: 1/3).

## Two bugs the test found (fixed after the measurement; not in the numbers)

1. **A bag added to a fare that includes one** (v01).
   - The customer chose the Azul fare, bag included, and said "com mala".
   - The booking added R$ 151 for a bag.
   - The customer, rightly, would not confirm.
   - **Fixed:** no bag is added to a fare with one.
   - **Suite:** `booking/bag-already-included`.
2. **"Reenviem o e-mail"** (v08).
   - `minha-reserva` answered with the status and "if you want, I ask the
     team to resend". It could not ask.
   - Each new message routed back to it, and the same answer came four
     times.
   - **Fixed:** when the customer says the e-mail did not arrive, the team
     is told to resend and the customer is told so. A resend request is
     no longer `minha-reserva`'s, and is in the handoff's list.
   - **Suite:** `agency/booking-status`.

## What three studies say

Each study measured on new personas. Each fixed what the previous test
found, and each gained less on the new test than on its dev set:

| Study | Goal (before → after) | Natural (before → after) |
| --- | --- | --- |
| agency-001 | 39% → 50% | 1.69 → 2.56 |
| agency-002 | 53% → 53% | 2.39 → 2.94 |
| agency-003 | 58% → 58% | 3.03 → 3.25 |

- **Across the three, the agent went from 39% to 58% of goals and from
  1.7 to 3.25 natural.** The personas differ, so these rows are not one
  scale. Within each study, before and after share them.
- **The pattern:** the long tail. Every new set of 12 customers brings
  new cases:
  - resend an e-mail;
  - "o mesmo uma semana depois";
  - two questions and a quote in one message;
  - insurance through the team.

  Each is fixed in minutes once seen. The next set brings others.
- **Safety held throughout:** 0 invariant failures in 216 test
  conversations.
- **Falsity did not drop below about a third of conversations.** Many
  flagged statements are:
  - the agent relaying what the team said (no system call confirms it);
  - rounding (R$ 181 for R$ 180,90).

## What was built

**Platform:**
- `wait.relay`;
- the rest of a reply answered, with the run's shared reads;
- a reply with a `format` question's pattern is its answer;
- `freeMode.whenUnsure`.

**Traveler:**
- extras read while booking, and the bag offered at its price;
- three relayed rounds with the team;
- `minha-reserva`;
- `whenUnsure` asks the team;
- area vs city clarified.
