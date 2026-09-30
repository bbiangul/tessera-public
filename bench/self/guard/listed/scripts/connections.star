# Connections that change airports (land at GRU, fly out of CGH): a port of
# the legacy platform's 04b_airport_change_risk (bash + jq), compared with the
# original on the same fixtures in scoring_test.go. The drive time comes
# from Google Routes (plugins/routes.yaml) in one route-matrix call; the
# original made one call per pair. Intended differences:
#
#   1. The result has the same shape when there is nothing to report; the
#      original left fields out.
#   2. No English warning text: the message is built in present.star.
#   3. "estimated" is true only when the drive time was not measured; the
#      original's `.estimated // true` made it always true (jq's // treats
#      false as missing).

# Airport coordinates for the route matrix: the original's table.
AIRPORTS = {
    "GRU": (-23.4356, -46.4731), "CGH": (-23.6261, -46.6564), "VCP": (-23.0074, -47.1345),
    "GIG": (-22.8099, -43.2505), "SDU": (-22.9104, -43.1631), "BSB": (-15.8711, -47.9186),
    "CNF": (-19.6244, -43.9719), "PLU": (-19.8512, -43.9506), "JFK": (40.6413, -73.7781),
    "LGA": (40.7769, -73.8740), "EWR": (40.6895, -74.1745), "LAX": (33.9416, -118.4085),
    "ORD": (41.9742, -87.9073), "MDW": (41.7868, -87.7522), "MIA": (25.7959, -80.2870),
    "LHR": (51.4700, -0.4543), "LGW": (51.1537, -0.1821), "STN": (51.8860, 0.2389),
    "LTN": (51.8747, -0.3683), "CDG": (49.0097, 2.5479), "ORY": (48.7262, 2.3652),
    "FRA": (50.0379, 8.5622), "AMS": (52.3105, 4.7683), "MAD": (40.4983, -3.5676),
    "BCN": (41.2974, 2.0833), "FCO": (41.8003, 12.2389), "CIA": (41.7994, 12.5949),
    "MXP": (45.6306, 8.7281), "LIN": (45.4497, 9.2783), "LIS": (38.7756, -9.1354),
    "NRT": (35.7720, 140.3929), "HND": (35.5494, 139.7798), "PVG": (31.1443, 121.8083),
    "SHA": (31.1979, 121.3363), "SIN": (1.3644, 103.9915), "ICN": (37.4602, 126.4407),
    "GMP": (37.5583, 126.7906), "DXB": (25.2528, 55.3644), "DWC": (24.8964, 55.1614),
    "EZE": (-34.8222, -58.5358), "AEP": (-34.5592, -58.4156), "BKK": (13.6900, 100.7501),
    "DMK": (13.9126, 100.6068),
}

UNKNOWN_TRANSFER = 90  # minutes, when the drive time is not known
BUFFER_MIN = 45        # below transfer + this: HIGH
BUFFER_STD = 75        # below transfer + this: MEDIUM

def _t(s):
    s = s.split(".")[0]  # the original dropped fractions of a second
    return time.parse_time(s + "Z")

def _whole(x):
    return int(x) if x == int(x) else x

def airport_changes(offers):
    """Every connection where the next leg leaves from another airport."""
    changes = []
    for o in offers or []:
        for si, seg in enumerate(o.get("segments") or []):
            legs = seg.get("legs") or []
            for i in range(len(legs) - 1):
                arrive_at = legs[i]["arrivalAirport"]["code"]
                depart_from = legs[i + 1]["departureAirport"]["code"]
                if arrive_at == depart_from:
                    continue
                arr, dep = legs[i]["arrivalTime"], legs[i + 1]["departureTime"]
                changes.append({
                    "flightToken": o.get("token"), "segmentIndex": si, "legIndex": i,
                    "fromAirport": arrive_at, "toAirport": depart_from,
                    "arrivalTime": arr, "departureTime": dep,
                    "layoverMinutes": _whole((_t(dep) - _t(arr)).minutes),
                })
    pairs = {}
    for c in changes:
        pairs[(c["fromAirport"], c["toAirport"])] = True
    tokens = {c["flightToken"]: True for c in changes}
    return {
        "airportChanges": changes,
        "uniquePairs": [{"from": p[0], "to": p[1]} for p in sorted(pairs.keys())],
        "totalChanges": len(changes),
        "affectedFlights": len(tokens),
    }

def _waypoint(code):
    lat, lng = AIRPORTS[code]
    return {"waypoint": {"location": {"latLng": {"latitude": lat, "longitude": lng}}}}

def matrix_request(detected):
    """The route-matrix question for the pairs with known coordinates:
    origins, destinations, and which cell answers which pair."""
    froms, tos, cells = [], [], []
    for p in detected["uniquePairs"]:
        if p["from"] not in AIRPORTS or p["to"] not in AIRPORTS:
            continue
        if p["from"] not in froms:
            froms.append(p["from"])
        if p["to"] not in tos:
            tos.append(p["to"])
        cells.append({"from": p["from"], "to": p["to"], "originIndex": froms.index(p["from"]), "destinationIndex": tos.index(p["to"])})
    return {
        "origins": [_waypoint(c) for c in froms],
        "destinations": [_waypoint(c) for c in tos],
        "cells": cells,
    }

def transfer_times(request, elements):
    """Drive minutes per pair from the route matrix's elements; a pair the
    matrix did not answer (or with no route) is estimated."""
    found = {}
    for e in elements or []:
        d = e.get("duration")
        if e.get("condition", "ROUTE_EXISTS") == "ROUTE_EXISTS" and d:
            found[(e.get("originIndex", 0), e.get("destinationIndex", 0))] = int(d.rstrip("s")) // 60
    out = []
    for c in request["cells"]:
        m = found.get((c["originIndex"], c["destinationIndex"]))
        if m == None:
            out.append({"from": c["from"], "to": c["to"], "transferMinutes": UNKNOWN_TRANSFER, "estimated": True})
        else:
            out.append({"from": c["from"], "to": c["to"], "transferMinutes": m, "estimated": False})
    return {"transferTimes": out}

def _unique_sorted(xs):
    return sorted({x: True for x in xs}.keys())

def assess(detected, transfers):
    """The original's rule: HIGH when the layover is shorter than the drive
    plus 45 minutes, MEDIUM when shorter than the drive plus 75."""
    changes = (detected or {}).get("airportChanges") or []
    times = (transfers or {}).get("transferTimes") or []
    out = []
    for c in changes:
        t = None
        for x in times:
            if x["from"] == c["fromAirport"] and x["to"] == c["toAirport"]:
                t = x
                break
        transfer = t["transferMinutes"] if t else UNKNOWN_TRANSFER
        layover = c["layoverMinutes"]
        if layover < transfer + BUFFER_MIN:
            level = "HIGH"
        elif layover < transfer + BUFFER_STD:
            level = "MEDIUM"
        else:
            level = "LOW"
        out.append({
            "flightToken": c["flightToken"], "fromAirport": c["fromAirport"], "toAirport": c["toAirport"],
            "layoverMinutes": layover, "transferMinutes": transfer, "marginMinutes": layover - transfer,
            "minRequired": transfer + BUFFER_MIN, "riskLevel": level,
            "estimated": t["estimated"] if t else True,
        })
    high = _unique_sorted([a["flightToken"] for a in out if a["riskLevel"] == "HIGH"])
    medium = _unique_sorted([a["flightToken"] for a in out if a["riskLevel"] == "MEDIUM"])
    return {
        "riskAssessments": out,
        "highRiskFlights": high,
        "mediumRiskFlights": medium,
        "summary": {"total": len(out), "highRisk": len(high), "mediumRisk": len(medium),
                    "lowRisk": len([a for a in out if a["riskLevel"] == "LOW"])},
    }

def risk(detected, request, elements):
    """Transfer times from the route matrix's answer (null when Routes
    failed: every drive is then estimated), assessed."""
    return assess(detected, transfer_times(request, elements))

def with_risk(ranked, risk):
    """The original's merge into the ranking: each flight gets its worst
    change; HIGH costs 25 points and MEDIUM 15, then the list is sorted
    again."""
    by_token = {}
    for a in risk["riskAssessments"]:
        by_token.setdefault(a["flightToken"], []).append(a)
    flights = []
    for f in ranked["topFlights"]:
        f = dict(f)
        risks = by_token.get(f["token"], [])
        level = None
        if risks:
            level = "LOW"
            if [a for a in risks if a["riskLevel"] == "HIGH"]:
                level = "HIGH"
            elif [a for a in risks if a["riskLevel"] == "MEDIUM"]:
                level = "MEDIUM"
        f["hasAirportChange"] = bool(risks)
        f["airportChangeRisk"] = level
        f["airportChanges"] = [{"from": a["fromAirport"], "to": a["toAirport"], "layover": a["layoverMinutes"],
                                "transfer": a["transferMinutes"], "margin": a["marginMinutes"]} for a in risks]
        f["score"] = f["score"] + {"HIGH": -25, "MEDIUM": -15}.get(level, 0)
        flights.append(f)
    out = dict(ranked)
    out["topFlights"] = sorted(flights, key = lambda f: -f["score"])
    return out
