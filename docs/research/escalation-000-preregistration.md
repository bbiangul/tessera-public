# Escalation 000: pre-registration

- **Date:** 2026-09-24, before any run on the test set.
- **Issue:** [#25](https://github.com/bbiangul/tessera/issues/25).
- **Why:** a real agent built as an LLM with tools asked a franchisee's
  coach without need in 7 of 14 runs:
  - The escalation decision and the answer share one prompt.
  - Every rewrite of the tool's description that fixed the escalation
    made the answer worse.
  - Telling signal from noise took 20 repetitions.

  The claim to test: making escalation a declared, calibrated decision,
  separate from the answer, does better. **Nothing in this run tests that
  yet.** It measures the phenomenon and today's Tessera, before
  anything is built.

## Material (synthetic, no customer data)

- `bench/escalation/checklist.json`: an invented franchise onboarding
  checklist, 28 short sections, about 800 words.
- `bench/escalation/questions.json`: 34 questions in the owners' style.
  Each has a label and the points a correct answer contains:
  - **answer:** the checklist settles it. Asking the coach is unneeded.
  - **coach:** a decision, approval or exception the checklist gives to
    the coach. The coach must be asked.
  - **partial:** the checklist gives the step to take now; a detail it
    does not state is settled by that step or does not block it. Asking
    the coach is unneeded. t17 is the real case's analogue: signing a
    lease before a license arrives.
- **Split:**
  - dev: 13 questions, which I may look at while building;
  - test: 21 questions (11 answer, 6 coach, 4 partial), never tuned on.

## Competitors

The same model for every text call, `deepseek/deepseek-v4.1-flash`
through OpenRouter, provider DeepSeek, reasoning off, temperature 0. Every
stack gets the same policy text and the checklist, and one way to reach
the coach.

- **plain:** a chat-completions tool loop, with the checklist in the
  system prompt and an `ask_coach` tool.
- **langgraph:** `create_agent`, the same prompt and tool.
- **agents-sdk:** `Agent` + `Runner.run`, the same prompt and tool.
- **tessera (today):** only features that exist now
  (`bench/escalation/tessera`):
  - the free mode answers from the checklist, a public read;
  - a routable `ask-coach` Function tells the owner what the checklist
    says and escalates to the `coach` queue;
  - the router (Jev) chooses between them from the message and the
    Function's description. **It does not see the checklist.**

## Runs

- The test set, 20 repeats per question and stack: 1,680 runs.
- One question per conversation.
- A run is **escalated** when the coach was asked: an `ask_coach` call,
  or a Tessera run waiting for the coach queue.

## Metrics

- **Unneeded escalation rate:** escalated runs on *answer* and *partial*
  questions, each label also reported apart.
- **Coach recall:** escalated runs on *coach* questions.
- **Flakiness:** the questions a stack escalates in some repeats but not
  all (0 < rate < 1), and the mean per-question standard deviation.
- **Answer correctness** on repeats 1–5 of every question and stack (420
  replies). The judge is `anthropic/claude-sonnet-5`, reasoning off. It
  sees the question, its label, the reference points and the reply, not
  which stack wrote it, and answers:
  - `correct`: the reply gives the reference points that matter, and
    nothing contradicts the checklist;
  - `false_claim`: the reply states something the checklist contradicts,
    or claims a decision that belongs to the coach.
- Cost and latency per run.
- Rates with 95% Wilson intervals.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| E1 | The phenomenon exists in LLM tool loops | At least one LLM stack escalates ≥ 10% of *answer* + *partial* runs |
| E2 | They do not miss what the coach must decide | Every LLM stack's coach recall ≥ 90% |
| E3 | Today's Tessera does **not** beat them on unneeded escalation: its router cannot see the checklist | Tessera's unneeded rate ≥ the best LLM stack's |
| E4 | LLM stacks are flaky on the borderline questions | At least 3 test questions with 0 < rate < 1 in some LLM stack |

E3 expects Tessera to lose here. The next step builds the decision this
benchmark is about, and is judged against this baseline on the same
test set.

**Budget:** $3 for runs and judge.
