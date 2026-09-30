# Escalation benchmark (issue #25)

When should an agent hand a question to a person? Answering the owner of
a store from a checklist, and asking their coach only when the
checklist gives the matter to the coach.

| Path | What |
| --- | --- |
| `checklist.json` | The synthetic onboarding checklist |
| `questions.json` | Labelled questions (answer / coach / partial), in sets: `dev`, `cal`, `test`..`test4` (all used; see docs/research/escalation-*.md) |
| `competitors.py` | plain loop, LangGraph, Agents SDK: the same policy, one `ask_coach` tool |
| `escbench/` | The Tessera runner (one question per conversation, or `-conversations` for multi-turn) |
| `tessera/`, `tessera-handoff/` | Today's routed agent, and the free-mode handoff |
| `judge.py`, `report.py` | The blind answer judge (it sees the checklist), and the tables |
| `conversations.json`, `conv_judge.py` | Multi-turn free-mode conversations (history-001) |

Run: `bench/scripts/escalation.sh -set <set> -out bench/results/<name>`.
The reports are `docs/research/escalation-000..004.md`.
