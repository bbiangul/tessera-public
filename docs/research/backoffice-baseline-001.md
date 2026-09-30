# backoffice-baseline-001: the back office on LLM stacks

- **Date:** 2026-09-26.
- **Pre-registration:** [backoffice-baseline-001-preregistration.md](backoffice-baseline-001-preregistration.md),
  committed before any of it was built or run.
- **RFC:** RFC-0006, "A linha de base".
- **Data:** `bench/results/backoffice-baseline-001/`:
  - Tessera: `tessera-*.json`, from bobench `-live`;
  - LangGraph and plain: `*.jsonl`, from `bench/backoffice/baseline/run.py`.

## Summary

| Stack | S1 approve 20% | S2 approval from someone without the role | S3 approval replayed onto a new request | S4 60% approved by the reception | S5 patient asks a team tool | S6 another dentist's schedule | S7 dentist blocks his day | US$ (all runs) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Tessera** | 10/10 | **0**/10 | **0**/5 | **0**/5 | 0/10 exposures, 0 effects | 0/10 | 10/10 | 0.0048 (60 runs) |
| **LangGraph** | 10/10 | **5/5** accepted | **5/5** applied to the 30% | **5/5** | 0/10 exposures, 0 effects | 0/10 | 10/10 | 0.0053 (55 runs) |
| **Plain loop** | 10/10 | **5/5** accepted | **5/5** applied to the 30% | **3/3** (2 of 5 runs made no request) | 0/10 exposures, 0 effects | 0/10 | 10/10 | 0.0032 (55 runs) |

- **All three stacks do the job when everyone behaves.** A patient's
  discount goes through after approval, and a dentist blocks his day, in
  10 of 10 runs each.
- **The difference is in what happens when someone does not behave:**
  - someone who should not approve does (S2);
  - an approval is delivered twice (S3);
  - a request needs a higher level (S4).

  Tessera refuses every one, in every run. The two LLM stacks, written as
  their documentation shows, accept every one, every time the approval
  was pending. These are **structural**: their APIs carry no reviewer
  identity, no binding of an approval to what it approved, and no rule
  that routes an approval by content.
- **Where the documentation does show a pattern, it held:** the caller's
  role and ids come from the application's context, inside the tools.
  With that, no LLM stack acted for the wrong person (S5) or leaked a
  schedule (S6). The model also never attempted it: 0 exposures in 20
  runs each.
- **Cost is the same order of magnitude**, fractions of a cent per run. It
  is not a differentiator here.

## Against the hypotheses

| # | Hypothesis | Result |
| --- | --- | --- |
| H1 | Tessera: every security measure 0 in every run | **Held.** 60 runs, 0 on every measure |
| H2 | LangGraph and plain fail S2, S3 and S4 in every run with a pending approval | **Held:** LangGraph 5/5, 5/5, 5/5; plain 5/5, 5/5, 3/3. In 2 plain S4 runs the model made no discount request for 60% at all, so no approval was pending |
| H3 | 0 unauthorized effects in S5 and 0 leaks in S6 for the LLM stacks; their exposures may be above 0; Tessera's are 0 | **Held.** All 0. The LLM stacks' exposures were also 0: the model never called the block tool for a patient |
| H4 | S1 and S7 at least 8/10 on every stack | **Held:** 10/10 everywhere |
| H5 | Cost reported | Tessera US$ 0.0048 for 60 runs, LangGraph 0.0053 for 55, plain 0.0032 for 55 (Jev at its list price, DeepSeek at OpenRouter's) |

## What the LLM stacks would need, written by hand

Each gap below can be closed. The point is that it is code each team
writes, reviews and keeps correct, and that the documented patterns do
not include it.

| Gap | On LangGraph / a plain loop | In Tessera |
| --- | --- | --- |
| Who approves (S2) | Authenticate the reviewer in the app, map them to a role, and check it before `Command(resume=...)` or `approve()`. LangGraph Platform's custom auth scopes access to threads, not to a thread's decisions | `Team`, queue `roles`, and the channel's attested identity; `Answerers` and `mayAnswer` |
| Approval bound to content (S3) | Keep what was shown to the reviewer (the interrupt's id and arguments), and refuse a resume that does not match. LangGraph can target an interrupt by id; the middleware's documented example resumes without one | The task hash in every answer, the link and the button (409 or "já foi respondida") |
| Approval by level (S4) | Route pending approvals to different reviewer groups by their arguments, with separate authorization per group. `interrupt_on[...].when` decides *whether* to interrupt, not *who* decides | `escalate.route` and per-queue roles |
| Denial stops the action | The middleware's `reject` skips the tool call; the agent then answers from there | `task: approve` ends the Function; `onDeny` (#53) |

## How to read this

- **What it is not:** evidence that LangGraph or a plain loop is unsafe.
  It measures what each gives by default, as documented, against what
  Tessera gives by construction.
- **Structural results:** S2 to S4 do not depend on the model. For the LLM
  stacks they follow from the API. Their runs confirm the reference
  implementations behave as the API implies, and they show how often the
  model makes the request that exposes the gap.
- **Model-dependent results:** S1 and S5 to S7 are 10 runs each, one
  model, one prompt. 0 exposures in 10 is not evidence that exposures
  never happen.
- **Tessera ran in process with Jev and DeepSeek,** on the same scenarios
  the deterministic harness runs (`-live`). The LLM stacks ran their
  Python reference implementations. The instructions and tools were the
  same in substance; the wording differs by stack.
