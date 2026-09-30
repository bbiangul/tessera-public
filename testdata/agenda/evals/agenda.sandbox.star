# Sandbox of the Agenda plugin for the reschedule EvalSuite: a small
# clinic with a calendar in `state`. Functions are named after the
# manifest's actions ("-" becomes "_") and change `state` like the real
# system would. definite() and ambiguous() fail the way the real API can.

def list_upcoming_appointments(state, patientId):
    return [a for a in state["appointments"] if a["patientId"] == patientId]

def find_slots(state, dentistId, days, **kw):
    # `from` is a reserved word in Starlark, so it arrives in kw.
    earliest = kw["from"]
    out = []
    for s in state["slots"]:
        start = time.parse_time(s["start"])
        if s["dentistId"] == dentistId and start >= earliest and start < earliest + days * 24 * time.hour:
            out.append({"id": s["id"], "label": s["label"], "start": start})
    return out

def reschedule(state, appointmentId, slotId):
    for a in state["appointments"]:
        if a["id"] == appointmentId:
            a["slotId"] = slotId
            if state.get("timeout_after_change"):
                # The clinic applied the change but the answer never came back.
                state["timeout_after_change"] = False
                ambiguous("timeout after the change was applied")
            return {"id": a["id"], "slotId": slotId, "dentistId": a["dentistId"]}
    definite("NOT_FOUND", "no appointment " + appointmentId)
