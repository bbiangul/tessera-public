# Authoring 003: external software-house kit

2026-09-24, base f1e5f56. Owner explicitly requires a new subagent with only
what an external partner receives: skill, docs, CLI, integration contract.
No history, source, hand-written clinic, prior authored clinic, or evaluator
feedback. The author starts with fork_turns=none and the default model;
record its actual model if available, otherwise do not invent one.

## Delivery surface and isolation

An independent directory /private/tmp/tessera-partner-authoring-003 contains
compiled CLI, the authoring skill, three product guides, generic travel/
franchise/outreach resource examples, a client brief and clinic OpenAPI.
No .git, engine source, research results, clinic definitions or test tasks.
The kit manifest hashes all initial files. Its AGENTS.md is a partner-facing
replacement, not repository contributor instructions. Effect classes are
pre-approved explicitly in the synthetic client's initial brief.

The collaboration service shares the filesystem. This is NOT a separate
machine or a global permission sandbox for every agent tool. The author is
instructed to access only the kit, and every shell invocation must use the
provided sandbox-exec profile, which blocks reads/writes of the repository
and reads of .codex. A denied source-read probe and successful CLI probe
verify that shell boundary. No other tools, outside access or binary
inspection are allowed. Record compliance/violations; do not claim malicious
agent-proof isolation. Zero previous turns are passed to the author.

CLI launcher logs commands and allows at most twenty live tests at $0.05
each, loading credentials without exposing them. No changing launcher,
profile or raw binary invocation. Files not included are unavailable; broken
research citations are recorded as kit gaps, not accessed elsewhere.

## Precommitted gates and evaluator

Author: one brief, <=60 minutes, zero functional interventions, all authored
validate/live/replay gates pass; report time, calls, iterations and doc gaps.
Questions are logged as interventions; infrastructure repairs are recorded
separately, never silently treated as authoring success. Do not send held-out
scores or implementation suggestions back to the author. Freeze its delivery
before evaluation. If gates fail, retain the artifact and measure it as-is.

Evaluator-only, inaccessible to author:
- new market tasks seed 41, 20 tasks x3 x C0-C3 =240 authored runs;
  C0 >=97%, fired C1-C3 >=95%, zero duplicate/unconfirmed/foreign effects.
- current hand-written tessera-context, same runtime/tasks, C0 60 runs.
  Blind naturalness judge compares authored vs current reference on 60
  matched task/repeat pairs, both orders. Eligible naturalness and clarity
  shares >=45% of decided consistent pairs each; report ties/disagreements
  and small-sample uncertainty, no statistical equivalence claim.
- fresh off-script seed 39, 60 authored runs: >=95% judged success, zero
  retained false claims, <=5% dead ends. Manually review every negative;
  raw judgments stay. No tuning on either evaluator set.

No runtime or skill fixes during authoring/evaluation. Findings become
backlog work after frozen results, not repairs to improve the score. No
claim that historical 55% market parity automatically applies here.

Budget $5: author $1; authored market $0.40; reference C0 $0.20;
pair judge $1.20; off-script $0.40; off-script judge $0.60; reserve $1.20.
Existing judge concurrency can modestly overshoot its guard; report actual
spend. All integration data synthetic; keys never logged or committed.
