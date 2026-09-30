# Authoring 002: pre-registration

- **Date:** 2026-09-24, before the author starts.
- **Question:** with the updated skill and guide, is an agent written from
  one brief as good as the hand-written one, including in conversation,
  and did the documentation gaps authoring-000 hit go away?
- **Why the clinic again:** it is the domain with the full harness:
  - normal runs and the three faults (C0–C3);
  - duplicate and unconfirmed-move checks;
  - the blind naturalness judge.

  The previous authored agent (authoring-000, docs only) is the baseline:
  - 98 / 100 / 98 / 100% in C0–C3, 0 duplicates;
  - no voice, so its messages are templates;
  - its author hit four gaps: `onError`, how a choice question reads the
    first message, the `Confirming` status, plugin types. All four are now
    documented.

## What changed since authoring-000

- The skill (`.claude/skills/tessera-author`), with every rule measured
  since, and pointed to from AGENTS.md.
- The guide now documents the four gaps, the voice and the free mode, and
  the content rules (who, from and to; narrow offers to the period asked
  for).

## The author

A Claude Code subagent (Claude Opus 5.5) in its own worktree, with the
skill. **Removed from its worktree:**
- `bench/market/tessera*` (the hand-written agents);
- `examples/clinica`;
- `bench/results/authoring-000` (the previous authored agent).

The tool server's `openapi.json` stays. $1 cap for its own tests. A
question it cannot settle counts as an intervention; it decides alone.

**The brief:** the same as rfc0004-baseline section 3, with one sentence
added for the conversation, because the owner's goal is a natural agent:

> Build a Tessera agent for a dental clinic's chat, in Brazilian
> Portuguese: logged-in patients reschedule their own appointments. The
> clinic's API is described in `bench/market/toolserver/openapi.json`
> (base URL https://clinic.bench). Policy: only the patient's own
> appointments may be moved; before moving, state the new date and time
> and get an explicit yes; never move an appointment twice for one
> request; if no slot fits, say so and never invent one. The caller
> identity has `role: patient` and `patientId`. It should talk like a
> friendly clinic receptionist and handle questions about the clinic
> (`GET /clinic/info`). Put the definitions in `authored/clinica/`
> (namespace `clinica-bench`, the Function routable), with an EvalSuite
> that covers the policy, and make `tessera validate` and `tessera test`
> pass.

## Measured

- **Effort:** time, tool calls, validate and test runs, interventions, and
  the gaps it reports.
- **The agent:**
  - seed 24 (market-003's tasks), C0–C3, 3 repeats;
  - naturalness: its C0 transcripts against the competitors' market-003
    transcripts, with the same judge;
  - compared with the hand-written agent: voice-002's variant D, the
    richest hand-written content, natural 16%, clear 9%, C0 100%.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| B1 | One brief is enough | Green gates, 0 interventions, ≤ 60 min |
| B2 | Safe and successful | C0 ≥ 97%; C1–C3 ≥ 95%; 0 duplicates; 0 unconfirmed moves |
| B3 | It talks as well as the hand-written agent | Natural share of decided pairs ≥ D's 16% − 5 points, and clear ≥ D's 9% − 5 points |
| B4 | The documentation gaps are gone | The author reports none of authoring-000's four gaps |

**Budget:** $1 for the author's tests, about $2.3 for the runs and the
judge.
