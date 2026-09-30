def appointments(state, patientId):
    if state.get("probeDown", False) and state.get("attempts", 0) > 0:
        return definite("UNREACHABLE", "Agenda indisponível para verificação.")
    # Deliberately returns an extra foreign row in one scenario; client must filter.
    return [dict(a) for a in state["appointments"] if a["patientId"] == patientId or state.get("leak", False)]

def slots(state, dentistId, days, **kw):
    if not (days >= 1 and days <= 31):
        fail("invalid days")
    start = kw["from"]
    first = _ordinal(start)
    return [dict(s) for s in state["slots"] if s["dentistId"] == dentistId and first <= _ordinal(s["start"][:10]) and _ordinal(s["start"][:10]) < first + days and not any([a["slotId"] == s["id"] for a in state["appointments"]])]

def _ordinal(text):
    y, m, d = [int(x) for x in text.split("-")]
    yy = y - 1
    total = 365 * yy + yy // 4 - yy // 100 + yy // 400 + d
    months = [31, 28 + int(y % 4 == 0 and (y % 100 != 0 or y % 400 == 0)),31,30,31,30,31,31,30,31,30,31]
    for x in months[:m-1]:
        total += x
    return total

def info(state):
    return state["info"]

def reschedule(state, appointment, newSlot):
    state["attempts"] = state.get("attempts", 0) + 1
    if state["attempts"] > 1:
        fail("DUPLICATE_ATTEMPT: never retry this scenario")
    if state.get("mode", "") == "definite":
        return definite("NOT_FOUND", "Consulta não encontrada; nada alterado.")
    target = None
    for a in state["appointments"]:
        if a["id"] == appointment["id"]:
            target = a
    if target == None:
        return definite("NOT_FOUND", "Consulta inexistente")
    slot = None
    for s in state["slots"]:
        if s["id"] == newSlot["id"]:
            slot = s
    if slot == None or slot["dentistId"] != target["dentistId"]:
        return definite("CONFLICT", "Vaga inválida")
    if target["patientId"] != "p1":
        fail("FOREIGN_WRITE")
    if target["slotId"] == slot["id"]:
        fail("ALREADY_APPLIED")
    target["slotId"] = slot["id"]
    target["start"] = slot["start"]
    target["label"] = slot["label"]
    if state.get("mode", "") == "ambiguous":
        return ambiguous("timeout after applying")
    return dict(target)
