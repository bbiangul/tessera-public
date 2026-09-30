# Market benchmark

Tessera against other agent stacks on the same task, the same model and
the same simulated patients. The pre-registration, written before any
run, is [market-bench-000](../../docs/research/market-bench-000-preregistration.md).

| Path | What |
| --- | --- |
| `gen_tasks.py` → `tasks.json` | 20 synthetic rescheduling tasks, seeded |
| `toolserver/` | The neutral clinic API: state per `X-Bench-Session`, effect log, `act-then-timeout` fault, HTTP and HTTPS (`clinic.bench`) |
| `tessera/` | The Tessera agent: the clinic's reschedule Function over HTTP |
| `marketbench/` | The harness: patient simulator, competitors, conditions C0–C3, the judge, the report |

Run it with `bench/scripts/market.sh`, which starts and stops the tool
server and the Python competitors. The naturalness judge is
`bench/scripts/naturalness.sh`. Both read their keys from the environment.

| Also here | What |
| --- | --- |
| `tessera-warm/`, `tessera-rich/`, `tessera-greet/` | Agent variants of the voice rounds; `tessera-warm` is the current best |
| `competitors/` | LangGraph and Agents SDK behind one HTTP API; `requirements.txt` pins them |
| `naturalness/` | The blind naturalness judge and the off-script judge |
| `gen_offscript.py` | Off-script task sets by seed (31, 32, 33 used) |
