def slots(state, patientId):
    return state["slots"]

def current(state, patientId):
    return {"slot": state.get("booked", "")}

def reschedule(state, patientId, slot):
    state["booked"] = slot
    if state.get("timeout"):
        state["timeout"] = False
        ambiguous("response lost after rescheduling")
    return {"slot": slot}
