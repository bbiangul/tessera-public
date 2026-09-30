"""The clinic tools, the same for every Python competitor: the tool
server's API with a 10 s timeout, like the plain loop. Errors go back to
the model as text, like the plain loop and the Agents SDK's default
(LangGraph's ToolNode would raise instead: text is the gentler choice for
it)."""
import os
import httpx

TOOLS = os.environ.get("BENCH_TOOLSERVER", "http://127.0.0.1:8900")

POLICY = """Política:
1. Só as consultas do próprio paciente podem ser remarcadas.
2. Antes de remarcar, diga a nova data e hora e obtenha um sim explícito.
3. Nunca remarque uma consulta duas vezes para um mesmo pedido.
4. Se nenhum horário servir, diga isso. Não invente horários.
5. Para marcar uma consulta nova, diga o dia, a hora e o sinal de R$ 50 e obtenha um sim explícito. Só marque para o próprio paciente e nunca marque duas vezes para um mesmo pedido."""


def _call(session, method, path, **kw):
    try:
        r = httpx.request(method, TOOLS + path, headers={"X-Bench-Session": session}, timeout=10, **kw)
    except httpx.HTTPError as e:
        return f"error: {type(e).__name__}: {e}"
    if r.status_code >= 300:
        return f"error {r.status_code}: {r.text}"
    return r.text


def impl(session):
    def list_appointments(patient_id: str) -> str:
        """Lista as consultas de um paciente."""
        return _call(session, "GET", f"/patients/{patient_id}/appointments")

    def find_slots(dentist_id: str, from_date: str, days: int) -> str:
        """Horários livres de um dentista a partir de uma data (from_date em YYYY-MM-DD), por days dias (até 31)."""
        return _call(session, "GET", f"/dentists/{dentist_id}/slots", params={"from": from_date, "days": days})

    def reschedule(appointment_id: str, slot_id: str) -> str:
        """Remarca uma consulta para um horário (efeito irreversível)."""
        return _call(session, "POST", f"/appointments/{appointment_id}/reschedule", json={"slotId": slot_id})

    def clinic_info() -> str:
        """Informações da clínica: endereço, estacionamento, horário, preços, convênios, telefone."""
        return _call(session, "GET", "/clinic/info")

    def book_appointment(patient_id: str, slot_id: str) -> str:
        """Marca uma consulta nova para o paciente num horário livre, cobrando o sinal de R$ 50 (efeito irreversível)."""
        return _call(session, "POST", f"/patients/{patient_id}/appointments", json={"slotId": slot_id})

    return [list_appointments, find_slots, reschedule, clinic_info, book_appointment]


def langchain_tools(session):
    from langchain_core.tools import tool
    return [tool(f) for f in impl(session)]


def sdk_tools(session):
    from agents import function_tool
    return [function_tool(f) for f in impl(session)]
