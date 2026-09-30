"""Escalation benchmark, LLM competitors (docs/research/escalation-000-preregistration.md).

One question per conversation. Each stack gets the same instructions (the
checklist in the system prompt) and one tool, ask_coach, and uses the same
model. A run records whether ask_coach was called and the final reply.

  python3 competitors.py --competitors plain,langgraph,agents-sdk --set test \
      --repeats 20 --out runs.jsonl [--max-cost 2] [--parallel 12]

Runs already in --out are kept and skipped, so a stopped run resumes.
"""
import argparse, json, os, threading, time, urllib.request
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
MODEL = os.environ.get("BENCH_MODEL", "deepseek/deepseek-v4.1-flash")
PROVIDER = os.environ.get("BENCH_PROVIDER", "DeepSeek")
EXTRA = {"provider": {"order": [PROVIDER], "allow_fallbacks": False}, "reasoning": {"enabled": False}}
KEY = os.environ["OPENROUTER_API_KEY"]
BASE = "https://openrouter.ai/api/v1"

COACH_REPLY = "Sent to the coach. They will answer the owner in the Owner Portal."
TOOL_DESC = "Ask the owner's coach: for a decision, an approval or an exception that belongs to the coach, or a process the coach starts."


def checklist_text():
    d = json.load(open(os.path.join(HERE, "checklist.json")))
    return "\n".join(f"## {s['title']}\n{s['text']}" for s in d["sections"])


# The same text is Tessera's free-mode instructions (tessera/agent.yaml).
POLICY = ("You are the onboarding assistant for Brightpath Learning store owners, in chat. "
          "Answer from the onboarding checklist. When the question needs a decision, an approval or an "
          "exception that the checklist gives to the owner's coach, or a process the coach starts, ask the coach and tell the owner you did. "
          "Do not ask the coach what the checklist already answers.")


def instructions():
    return POLICY + "\n\nCHECKLIST:\n" + checklist_text()


def list_price():
    d = json.load(urllib.request.urlopen(urllib.request.Request(BASE + "/models")))
    for m in d["data"]:
        if m["id"] == MODEL:
            return float(m["pricing"]["prompt"]), float(m["pricing"]["completion"])
    raise SystemExit("model not listed: " + MODEL)


class Plain:
    """The plain tool-calling loop: chat completions with tools, at most 4 rounds."""

    def __init__(self):
        from openai import OpenAI
        self.client = OpenAI(base_url=BASE, api_key=KEY, timeout=90)
        self.tools = [{"type": "function", "function": {"name": "ask_coach", "description": TOOL_DESC, "parameters": {
            "type": "object", "properties": {"question": {"type": "string", "description": "what the coach should decide"}},
            "required": ["question"]}}}]

    def run(self, text):
        msgs = [{"role": "system", "content": instructions()}, {"role": "user", "content": text}]
        asked, tin, tout, calls = 0, 0, 0, 0
        for _ in range(4):
            r = self.client.chat.completions.create(model=MODEL, messages=msgs, tools=self.tools, temperature=0, extra_body=EXTRA)
            calls += 1
            tin += r.usage.prompt_tokens
            tout += r.usage.completion_tokens
            m = r.choices[0].message
            msgs.append(m.model_dump(exclude_none=True))
            if not m.tool_calls:
                return m.content or "", asked, tin, tout, calls
            for tc in m.tool_calls:
                asked += tc.function.name == "ask_coach"
                msgs.append({"role": "tool", "tool_call_id": tc.id, "content": COACH_REPLY})
        return "(stopped after 4 rounds)", asked, tin, tout, calls


def ask_coach_impl(counter):
    def ask_coach(question: str) -> str:
        """Ask the owner's coach: for a decision, an approval or an exception that belongs to the coach, or a process the coach starts."""
        counter[0] += 1
        return COACH_REPLY
    return ask_coach


class LangGraph:
    """LangChain's create_agent (LangGraph 1.x), no checkpointer: one turn."""

    def __init__(self):
        from langchain_openai import ChatOpenAI
        self.model = ChatOpenAI(model=MODEL, base_url=BASE, api_key=KEY, temperature=0, extra_body=EXTRA, timeout=90)

    def run(self, text):
        from langchain.agents import create_agent
        from langchain_core.tools import tool
        counter = [0]
        graph = create_agent(self.model, [tool(ask_coach_impl(counter))], system_prompt=instructions())
        out = graph.invoke({"messages": [{"role": "user", "content": text}]})
        ai = [m for m in out["messages"] if m.type == "ai"]
        usage = [m.usage_metadata or {} for m in ai]
        reply = next((m.content for m in reversed(ai) if m.content and not getattr(m, "tool_calls", None)), "")
        return reply, counter[0], sum(u.get("input_tokens", 0) for u in usage), sum(u.get("output_tokens", 0) for u in usage), len(ai)


class AgentsSDK:
    """An Agent with a function tool and Runner.run, on one event loop."""

    def __init__(self):
        import asyncio
        from agents import set_tracing_disabled
        from openai import AsyncOpenAI
        set_tracing_disabled(True)  # tracing would send transcripts to OpenAI
        self.loop = asyncio.new_event_loop()
        threading.Thread(target=self.loop.run_forever, daemon=True).start()
        self.client = AsyncOpenAI(base_url=BASE, api_key=KEY, timeout=90)

    def run(self, text):
        import asyncio
        from agents import Agent, ModelSettings, OpenAIChatCompletionsModel, Runner, function_tool
        counter = [0]
        agent = Agent(name="Onboarding", instructions=instructions(), tools=[function_tool(ask_coach_impl(counter))],
                      model=OpenAIChatCompletionsModel(model=MODEL, openai_client=self.client),
                      model_settings=ModelSettings(temperature=0, extra_body=EXTRA))
        r = asyncio.run_coroutine_threadsafe(Runner.run(agent, text), self.loop).result()
        u = r.context_wrapper.usage
        return str(r.final_output), counter[0], u.input_tokens, u.output_tokens, u.requests


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--competitors", default="plain,langgraph,agents-sdk")
    ap.add_argument("--set", default="test")
    ap.add_argument("--repeats", type=int, default=20)
    ap.add_argument("--out", required=True)
    ap.add_argument("--max-cost", type=float, default=2.0)
    ap.add_argument("--parallel", type=int, default=12)
    a = ap.parse_args()
    pin, pout = list_price()
    qs = [q for q in json.load(open(os.path.join(HERE, "questions.json")))["questions"] if q["set"] == a.set]
    stacks = {"plain": Plain, "langgraph": LangGraph, "agents-sdk": AgentsSDK}
    done = set()
    if os.path.exists(a.out):
        for l in open(a.out):
            r = json.loads(l)
            done.add((r["competitor"], r["question"], r["repeat"]))
    jobs = [(c, q, k) for c in a.competitors.split(",") for k in range(1, a.repeats + 1) for q in qs if (c, q["id"], k) not in done]
    impl = {c: stacks[c]() for c in a.competitors.split(",")}
    spent, lock, f = [0.0], threading.Lock(), open(a.out, "a")

    def one(job):
        c, q, k = job
        with lock:
            if spent[0] >= a.max_cost:
                return
        t0 = time.time()
        try:
            reply, asked, tin, tout, calls = impl[c].run(q["text"])
            err = ""
        except Exception as e:  # recorded as the run's error
            reply, asked, tin, tout, calls, err = "", 0, 0, 0, 0, f"{type(e).__name__}: {e}"
        cost = tin * pin + tout * pout
        rec = {"competitor": c, "question": q["id"], "label": q["label"], "repeat": k, "escalated": asked > 0, "askCalls": asked,
               "reply": reply, "error": err, "latencyMs": int((time.time() - t0) * 1000), "calls": calls,
               "inputTokens": tin, "outputTokens": tout, "costUSD": cost}
        with lock:
            spent[0] += cost
            f.write(json.dumps(rec) + "\n")
            f.flush()

    with ThreadPoolExecutor(a.parallel) as ex:
        list(ex.map(one, jobs))
    print(f"{len(jobs)} runs, spent ${spent[0]:.4f}")


if __name__ == "__main__":
    main()
