# Sandbox of Google Routes' route matrix: drive times between the airports
# the suite knows, in the real response shape (one element per cell,
# durations as "<seconds>s").

_CODES = {-23.4356: "GRU", -23.6261: "CGH", 51.47: "LHR", 51.1537: "LGW"}

def _code(w):
    return _CODES.get(w["waypoint"]["location"]["latLng"]["latitude"], "?")

def drive_matrix(state, origins, destinations):
    state["calls"] = state.get("calls", 0) + 1
    if state.get("down"):
        definite("UNAVAILABLE", "routes is down")
    out = []
    for i, o in enumerate(origins):
        for j, d in enumerate(destinations):
            minutes = state["minutes"].get(_code(o) + "-" + _code(d))
            if minutes == None:
                out.append({"originIndex": i, "destinationIndex": j, "condition": "ROUTE_NOT_FOUND"})
            else:
                out.append({"originIndex": i, "destinationIndex": j, "condition": "ROUTE_EXISTS", "duration": "%ds" % (minutes * 60)})
    return out
