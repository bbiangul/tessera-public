"""Python competitors of the market benchmark
(docs/research/market-bench-000-preregistration.md): LangGraph and the
OpenAI Agents SDK, each written the way its docs show, behind one small
HTTP API the harness drives:

  POST /session {session, patientId, patientName}  -> {}
  POST /turn    {session, text}                    -> {replies, inputTokens, outputTokens, calls}
  POST /crash   {session}                          -> {}  the session's process-held state is gone

Every framework calls the same tools (tools.py) with the same model. A
crash drops what the framework keeps in memory and keeps what its
documented persistence keeps (LangGraph's SqliteSaver: a file).
"""
import argparse, json, os, sqlite3, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import tools

MODEL = os.environ.get("BENCH_MODEL", "deepseek/deepseek-v4.1-flash")
PROVIDER = os.environ.get("BENCH_PROVIDER", "DeepSeek")
EXTRA = {"provider": {"order": [PROVIDER], "allow_fallbacks": False}, "reasoning": {"enabled": False}}
KEY = os.environ["OPENROUTER_API_KEY"]
BASE = "https://openrouter.ai/api/v1"


def instructions(name, pid):
    return (f"Você é a recepção de uma clínica odontológica, no chat. Paciente logado: {name} (id {pid}). "
            f"Hoje é sexta-feira, 02/10/2026 (America/Sao_Paulo).\n{tools.POLICY}")


class LangGraph:
    """LangChain's create_agent (LangGraph 1.x moved create_react_agent
    there) with a checkpointer: InMemorySaver (the docs' examples) or
    SqliteSaver (the docs' recommendation beyond prototypes), one thread
    per conversation. No middleware: none is on by default."""

    def __init__(self, sqlite_dir=None):
        from langchain_openai import ChatOpenAI
        self.sqlite_dir = sqlite_dir
        self.model = ChatOpenAI(model=MODEL, base_url=BASE, api_key=KEY, temperature=0, extra_body=EXTRA, timeout=90)

    def new(self, session, name, pid):
        from langchain.agents import create_agent
        if self.sqlite_dir:
            from langgraph.checkpoint.sqlite import SqliteSaver
            conn = sqlite3.connect(os.path.join(self.sqlite_dir, f"{session}.sqlite"), check_same_thread=False)
            saver = SqliteSaver(conn)
        else:
            from langgraph.checkpoint.memory import InMemorySaver
            saver = InMemorySaver()
        graph = create_agent(self.model, tools.langchain_tools(session), system_prompt=instructions(name, pid), checkpointer=saver)
        return {"graph": graph, "config": {"configurable": {"thread_id": session}}}

    def turn(self, s, text):
        out = s["graph"].invoke({"messages": [{"role": "user", "content": text}]}, s["config"])
        msgs = out["messages"]
        # The new messages: after the last human message.
        i = max(k for k, m in enumerate(msgs) if m.type == "human")
        new = msgs[i + 1:]
        usage = [m.usage_metadata or {} for m in new if m.type == "ai"]
        replies = [m.content for m in new if m.type == "ai" and m.content and not getattr(m, "tool_calls", None)]
        return replies, sum(u.get("input_tokens", 0) for u in usage), sum(u.get("output_tokens", 0) for u in usage), len(usage)


class AgentsSDK:
    """An Agent with function tools and Runner.run, the conversation in a
    SQLiteSession (the docs' way to keep history; in memory by default).
    The SDK is async: every turn runs on one long-lived event loop, which
    the async OpenAI client is bound to."""

    def __init__(self):
        import asyncio
        from agents import set_tracing_disabled
        from openai import AsyncOpenAI
        set_tracing_disabled(True)  # tracing would send transcripts to OpenAI
        self.loop = asyncio.new_event_loop()
        threading.Thread(target=self.loop.run_forever, daemon=True).start()
        self.client = AsyncOpenAI(base_url=BASE, api_key=KEY, timeout=90)

    db = None  # a file for the persistent variant; in memory otherwise

    def new(self, session, name, pid):
        from agents import Agent, ModelSettings, OpenAIChatCompletionsModel, SQLiteSession
        agent = Agent(name="Recepção", instructions=instructions(name, pid), tools=tools.sdk_tools(session),
                      model=OpenAIChatCompletionsModel(model=MODEL, openai_client=self.client),
                      model_settings=ModelSettings(temperature=0, extra_body=EXTRA))
        store = SQLiteSession(session, self.db) if self.db else SQLiteSession(session)
        return {"agent": agent, "session": store}

    def turn(self, s, text):
        import asyncio
        from agents import Runner
        r = asyncio.run_coroutine_threadsafe(Runner.run(s["agent"], text, session=s["session"]), self.loop).result()
        u = r.context_wrapper.usage
        return [str(r.final_output)], u.input_tokens, u.output_tokens, u.requests



class CrewAI:
    """A single CrewAI Agent without a crew, the way its docs show:
    Agent(role, goal, backstory, tools, llm) and Agent.kickoff(messages)
    with the conversation as the list of messages (market-bench-005). The
    docs keep no conversation for a direct agent, so it lives in the
    session (in memory: a crash loses it). Telemetry is off: CrewAI sends
    it to its servers by default, and transcripts must not leave."""

    def __init__(self):
        os.environ["CREWAI_DISABLE_TELEMETRY"] = "true"
        os.environ["CREWAI_DISABLE_TRACKING"] = "true"
        os.environ["CREWAI_TRACING_ENABLED"] = "false"
        os.environ["OTEL_SDK_DISABLED"] = "true"
        from crewai import LLM
        self.llm = LLM(model="openrouter/" + MODEL, api_key=KEY, temperature=0, extra_body=EXTRA, timeout=90)

    def new(self, session, name, pid):
        from crewai import Agent
        from crewai.tools import tool
        agent = Agent(role="Recepção da clínica odontológica", goal="Atender o paciente no chat seguindo a política da clínica.",
                      backstory=instructions(name, pid), tools=[tool(f) for f in tools.impl(session)], llm=self.llm, verbose=False)
        return {"agent": agent, "history": []}

    def turn(self, s, text):
        s["history"].append({"role": "user", "content": text})
        out = s["agent"].kickoff(list(s["history"]))
        reply = str(out.raw)
        s["history"].append({"role": "assistant", "content": reply})
        u = out.usage_metrics or {}
        return [reply], u.get("prompt_tokens", 0), u.get("completion_tokens", 0), u.get("successful_requests", 0)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--framework", required=True, choices=["langgraph", "langgraph-sqlite", "agents-sdk", "agents-sdk-sqlite", "crewai"])
    ap.add_argument("--port", type=int, required=True)
    ap.add_argument("--data", default="/tmp")
    a = ap.parse_args()
    def sdk_file():
        s = AgentsSDK()
        s.db = os.path.join(a.data, "agents-sdk-sessions.db")  # the docs' persistent SQLiteSession
        return s

    fw = {"langgraph": lambda: LangGraph(), "langgraph-sqlite": lambda: LangGraph(a.data), "agents-sdk": AgentsSDK,
          "agents-sdk-sqlite": sdk_file, "crewai": CrewAI}[a.framework]()
    sessions, info, lock = {}, {}, threading.Lock()

    class H(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])) or b"{}")
            try:
                sid = body["session"]
                if self.path == "/session":
                    with lock:
                        info[sid] = (body["patientName"], body["patientId"])
                        sessions[sid] = fw.new(sid, body["patientName"], body["patientId"])
                    out = {}
                elif self.path == "/crash":
                    with lock:  # a new process: build the agent again; only persisted state survives
                        sessions[sid] = fw.new(sid, *info[sid])
                    out = {}
                elif self.path == "/turn":
                    replies, i, o, calls = fw.turn(sessions[sid], body["text"])
                    out = {"replies": replies, "inputTokens": i, "outputTokens": o, "calls": calls}
                else:
                    self.send_error(404)
                    return
                code = 200
            except Exception as e:  # reported to the harness as the run's error
                out, code = {"error": f"{type(e).__name__}: {e}"}, 500
            data = json.dumps(out).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

    ThreadingHTTPServer(("127.0.0.1", a.port), H).serve_forever()


if __name__ == "__main__":
    main()
