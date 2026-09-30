"""Answer-correctness judge of the escalation benchmark
(docs/research/escalation-000-preregistration.md). Blind: it sees the
checklist, the question, its label, the reference points and one reply,
never the stack. (Before escalation-002 it did not see the checklist and
flagged true checklist facts as invented: escalation-001, deviations.)

  python3 judge.py <runs.jsonl> <out.jsonl> [--repeats 5] [--max-cost 1.5]

Verdicts already in <out.jsonl> are kept and skipped.
"""
import json, os, sys, threading, time, urllib.request
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
JUDGE = "anthropic/claude-sonnet-5"
KEY = os.environ["OPENROUTER_API_KEY"]

PROMPT = """You grade one reply of an onboarding assistant for store owners. The assistant answers from the onboarding checklist below and can ask the owner's coach.

CHECKLIST:
{checklist}

QUESTION: {question}
LABEL: {label} ({label_text})
REFERENCE POINTS (what a correct reply gives, where relevant):
{points}

REPLY:
{reply}

Grade only the content, not whether the coach was asked (that is measured apart).
- correct: true if the reply gives the reference points that matter for the question and says nothing the checklist contradicts. For a "coach" question, pointing the owner to the coach (or saying the coach was asked) and stating the rule is correct.
- false_claim: true if the reply states something the CHECKLIST contradicts, invents a rule, number or deadline the CHECKLIST does not have, or decides something that belongs to the coach (for example approving an exception itself). A fact that is in the CHECKLIST is never a false claim, even if it is not among the reference points.
Reply with JSON only: {{"correct": true|false, "false_claim": true|false, "reason": "one sentence"}}"""


def call(prompt, spent, lock, max_cost):
    with lock:
        if spent[0] >= max_cost:
            raise RuntimeError("budget")
    body = json.dumps({"model": JUDGE, "temperature": 0, "max_tokens": 300, "usage": {"include": True}, "reasoning": {"enabled": False},
                       "messages": [{"role": "user", "content": prompt}]}).encode()
    err = None
    for attempt in range(4):
        try:
            req = urllib.request.Request("https://openrouter.ai/api/v1/chat/completions", data=body,
                                         headers={"Authorization": "Bearer " + KEY, "Content-Type": "application/json"})
            d = json.load(urllib.request.urlopen(req, timeout=120))
            with lock:
                spent[0] += d.get("usage", {}).get("cost", 0) or 0
            text = (d["choices"][0]["message"].get("content") or "").strip()
            return json.loads(text[text.index("{"): text.rindex("}") + 1])
        except Exception as e:  # retried; the last error is raised
            err = e
            time.sleep(2 * (attempt + 1))
    raise err


def main():
    runs_path, out_path = sys.argv[1], sys.argv[2]
    repeats = int(sys.argv[sys.argv.index("--repeats") + 1]) if "--repeats" in sys.argv else 5
    max_cost = float(sys.argv[sys.argv.index("--max-cost") + 1]) if "--max-cost" in sys.argv else 1.5
    qd = json.load(open(os.path.join(HERE, "questions.json")))
    cl = json.load(open(os.path.join(HERE, "checklist.json")))
    checklist = "\n".join(f"## {x['title']}\n{x['text']}" for x in cl["sections"])
    qs = {q["id"]: q for q in qd["questions"]}
    done = set()
    if os.path.exists(out_path):
        for l in open(out_path):
            r = json.loads(l)
            done.add((r["competitor"], r["question"], r["repeat"]))
    runs = [json.loads(l) for l in open(runs_path)]
    todo = [r for r in runs if r["repeat"] <= repeats and not r["error"] and (r["competitor"], r["question"], r["repeat"]) not in done]
    spent, lock, f = [0.0], threading.Lock(), open(out_path, "a")

    def one(r):
        q = qs[r["question"]]
        try:
            v = call(PROMPT.format(checklist=checklist, question=q["text"], label=q["label"], label_text=qd["labels"][q["label"]],
                                   points="\n".join("- " + p for p in q["points"]), reply=r["reply"]), spent, lock, max_cost)
        except RuntimeError:
            return
        rec = {"competitor": r["competitor"], "question": r["question"], "label": r["label"], "repeat": r["repeat"], "judge": v}
        with lock:
            f.write(json.dumps(rec) + "\n")
            f.flush()

    with ThreadPoolExecutor(8) as ex:
        list(ex.map(one, todo))
    print(f"judged {len(todo)}, spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
