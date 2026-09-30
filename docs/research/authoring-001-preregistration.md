# Authoring 001 (RFC-0004 part C): pre-registration

- **Date:** 2026-09-24, before either author starts.
- **Question:** with the authoring skill
  (`.claude/skills/tessera-author`), does one short brief to a coding
  agent produce a Tessera agent as good as the hand-written one, with no
  human help? And does the skill matter, compared with the docs alone?
- **Baseline:** [rfc0004-baseline](rfc0004-baseline.md), section 3. On the
  clinic, without the skill: 5.7 minutes, 0 interventions, an agent at
  98–100%. That task had a worked example nearby. This one is a new
  domain whose only precedent is hidden.

## The task

The escalation benchmark's domain: an onboarding assistant for franchise
owners, which answers from a checklist and asks the coach when needed.
Its hand-written agent (`bench/escalation/tessera-handoff`) is measured
on `test4` in [escalation-004](escalation-004-preregistration.md).

**The brief, verbatim, to both authors:**

> Build a Tessera agent, in English, for Brightpath Learning franchise
> owners who chat with it during onboarding. It answers their questions
> from the onboarding checklist, and asks the owner's coach when a
> question needs something only the coach can give: a decision, an
> approval, an exception, or a process the coach starts. It must not
> bother the coach with what the checklist already answers, and it never
> changes anything itself. The checklist is available as a read action
> `checklist/read` (plugin `checklist`, no input, output `map`; the
> benchmark serves it in process, so declare it without `http`); its
> content is `bench/escalation/checklist.json`. The coach is a human
> queue named `coach`. Namespace `brightpath`. Put the definitions in
> `authored/onboarding/`, with an EvalSuite, and make `tessera validate`
> and `tessera test` pass.

## Authors

Two Claude Code subagents (Claude Opus 5.5, this session's model), each in
its own git worktree, started at the same time:
- **with-skill:** told to follow `.claude/skills/tessera-author/SKILL.md`;
- **docs-only:** the skill file deleted in its worktree, and the pointer
  in AGENTS.md removed; the guides stay.

**Hidden from both** (not to be read):
- `bench/escalation/` except `checklist.json`;
- `bench/results/escalation-*`;
- `docs/research/escalation-*`.

They may call Jev and DeepSeek for their own tests, with at most $1 each.
A question they cannot settle alone counts as one human intervention and
is answered only with "decide yourself".

## Measured

- **Effort:** wall time, tool calls, validate and test runs, interventions.
- **The agent:**
  - `test4` through the same harness (`escbench`), 20 repeats (420 runs
    each), and the same judge on repeats 1–5;
  - against the hand-written agent's escalation-004 numbers, run by the
    same code.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| A1 | One brief is enough | The with-skill author ends with validate and test green, 0 interventions, within 60 minutes |
| A2 | The authored agent is as good as the hand-written one | With-skill: unneeded escalation ≤ hand-written + 5 points, recall ≥ hand-written − 5 points, correct ≥ hand-written − 5 points |
| A3 | The skill matters | With-skill beats docs-only on at least one of unneeded escalation, recall or correct by ≥ 10 points, and is not worse on the others by more than 5 |

A3 may well fail: the authoring guide already carries most of the
lessons, and it is available to both authors.

**Budget:** $1 per author for their own tests, plus about $1.2 for the
two benchmark runs and judges.
