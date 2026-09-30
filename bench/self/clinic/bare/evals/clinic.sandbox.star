# Sandbox of the clinic plugin for self-001: a fixed schedule and info.

def info(state):
    return state["info"]

def schedule(state, dentistId):
    return state["schedules"].get(dentistId, [])

def discount(state, patientId, percent):
    state["discounts"].append({"patientId": patientId, "percent": percent})
    return {"ok": True}

def block(state, dentistId, day):
    state["blocks"].append({"dentistId": dentistId, "day": day})
    return {"ok": True}

def discounts_report(state):
    return ["%s: %d%%" % (d["patientId"], d["percent"]) for d in state["discounts"]]
