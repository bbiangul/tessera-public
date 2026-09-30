"""Judge of the multi-turn free-mode conversations (issue #27, history-001).

For each turn it sees the checklist, the conversation so far and the reply,
and says whether the reply answers the turn correctly given the earlier
turns. Greetings after the first agent message are counted without a model.

  python3 conv_judge.py <runs.jsonl> <out.jsonl> [--max-cost 1]
"""
import json, os, re, sys, threading
from concurrent.futures import ThreadPoolExecutor

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from judge import call  # noqa: E402  (the same model, settings and budget handling)

HERE = os.path.dirname(os.path.abspath(__file__))
GREETING = re.compile(r"^\W*(hi|hello|hey|good (morning|afternoon|evening))\b", re.I)

PROMPT = """You grade one reply of an onboarding assistant for store owners, in a conversation. It answers from the checklist below.

CHECKLIST:
{checklist}

CONVERSATION SO FAR:
{history}

OWNER'S LAST MESSAGE: {turn}
REFERENCE POINTS: {points}

REPLY:
{reply}

- correct: true if the reply answers the owner's last message as meant in the conversation (a follow-up such as "and how much is it?" refers to the earlier topic) and gives the reference points, with nothing the checklist contradicts.
Reply with JSON only: {{"correct": true|false, "reason": "one sentence"}}"""


def main():
    runs_path, out_path = sys.argv[1], sys.argv[2]
    max_cost = float(sys.argv[sys.argv.index("--max-cost") + 1]) if "--max-cost" in sys.argv else 1.0
    cl = json.load(open(os.path.join(HERE, "checklist.json")))
    checklist = "\n".join(f"## {x['title']}\n{x['text']}" for x in cl["sections"])
    points = {c["id"]: c["points"] for c in json.load(open(os.path.join(HERE, "conversations.json")))["conversations"]}
    runs = [json.loads(l) for l in open(runs_path)]
    spent, lock, f = [0.0], threading.Lock(), open(out_path, "a")
    jobs = []
    for r in runs:
        for i, turn in enumerate(r["turns"]):
            if i < len(r["replies"]):
                jobs.append((r, i))

    def one(job):
        r, i = job
        history = []
        for k in range(i):
            history.append("Owner: " + r["turns"][k])
            history.append("Assistant: " + " / ".join(r["replies"][k]))
        reply = " / ".join(r["replies"][i])
        try:
            v = call(PROMPT.format(checklist=checklist, history="\n".join(history) or "(none)", turn=r["turns"][i],
                                   points="; ".join(points[r["conversation"]][i]), reply=reply), spent, lock, max_cost)
        except RuntimeError:
            return
        greets = i > 0 and any(GREETING.match(m) for m in r["replies"][i])
        rec = {"competitor": r["competitor"], "conversation": r["conversation"], "repeat": r["repeat"], "turn": i,
               "greetsAgain": greets, "judge": v}
        with lock:
            f.write(json.dumps(rec) + "\n")
            f.flush()

    with ThreadPoolExecutor(8) as ex:
        list(ex.map(one, jobs))
    print(f"judged {len(jobs)}, spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
