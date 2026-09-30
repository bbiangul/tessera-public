# Flight scoring for the traveler agent: a port of the legacy platform's scoreFlights
# (bash + jq). The parity test (examples/traveler/scoring_test.go) runs this
# on the same fixtures as the original and compares the outputs; the only
# differences are the fixes listed there:
#
#   1. Avoided airlines also match by name ("Gol" avoids G3). The original
#      compared the typed name with IATA codes, so a name never matched.
#   2. loyaltyProgram has no trailing newline (echo | jq -Rs added one).
#   3. When every flight is filtered out the result is still valid (the
#      original produced invalid JSON).
#
# Starlark has no regular expressions, so the original's grep/sed patterns
# are written as explicit scans; the tests cover each of them.

# --- text matching, as the original's `grep -qi` patterns -------------------

def _matches_at(text, i, pattern):
    # "." in the original patterns matches any one character.
    if i + len(pattern) > len(text):
        return False
    for j in range(len(pattern)):
        if pattern[j] != "." and text[i + j] != pattern[j]:
            return False
    return True

def _contains(text, pattern):
    for i in range(len(text) - len(pattern) + 1):
        if _matches_at(text, i, pattern):
            return True
    return False

def _any(text, patterns):
    for p in patterns:
        if _contains(text, p):
            return True
    return False

LOYALTY_PROGRAMS = [
    (["smiles", "gol"], ["G3"]),
    (["latam", "multiplus"], ["LA", "JJ"]),
    (["tudoazul", "azul"], ["AD"]),
    (["aadvantage", "american"], ["AA"]),
    (["mileageplus", "united"], ["UA"]),
    (["skymiles", "delta"], ["DL"]),
    (["flying.blue", "air.france", "klm"], ["AF", "KL"]),
    (["miles.more", "lufthansa"], ["LH"]),
    (["executive.club", "british"], ["BA"]),
    (["tap", "miles.go"], ["TP"]),
    (["avianca", "lifemiles"], ["AV"]),
    (["copa", "connectmiles"], ["CM"]),
]

PREFERENCE_WORDS = ["prefer", "prefiro", "rather", "priority", "prioritario"]

def _preference(program):
    # sed -E 's/.*(prefer|...)[^a-z]*//i': everything after the LAST
    # preference word, without the non-letters that follow it.
    best, best_end = -1, -1
    for w in PREFERENCE_WORDS:
        i = program.rfind(w)
        if i > best or (i == best and i + len(w) > best_end):
            best, best_end = i, i + len(w)
    if best < 0:
        return ""
    rest = program[best_end:]
    k = 0
    for k in range(len(rest) + 1):
        if k == len(rest) or rest[k].isalpha():
            break
    return rest[k:]

def loyalty(program):
    """Returns (loyalty codes, preferred codes) for a loyalty program text."""
    program = program.lower()
    pref = _preference(program) if _any(program, PREFERENCE_WORDS) else ""
    codes, preferred = [], []
    for patterns, airlines in LOYALTY_PROGRAMS:
        if _any(program, patterns):
            codes.extend(airlines)
            if _any(pref, patterns):
                preferred = airlines
    return codes, preferred

# --- time constraints --------------------------------------------------------

def _first_time(text):
    # grep -oE '[0-9]{1,2}[h:]?[0-9]{0,2}' | head -1, then sed 's/h/:00/;s/::/:/'
    for i in range(len(text)):
        if text[i].isdigit():
            j = i
            for _ in range(2):
                if j < len(text) and text[j].isdigit():
                    j += 1
            if j < len(text) and text[j] in "h:":
                j += 1
            for _ in range(2):
                if j < len(text) and text[j].isdigit():
                    j += 1
            s = text[i:j].replace("h", ":00", 1).replace("::", ":", 1)
            return s if ":" in s else s + ":00"
    return ""

def _minutes(hhmm):
    parts = hhmm.split(":")
    minutes = int(parts[0]) * 60
    if len(parts) > 1 and parts[1].isdigit():
        minutes += int(parts[1])
    return minutes

def time_constraint(text):
    t = text.lower().strip()
    c = {"preference": "", "arriveBy": "", "departAfter": "", "noOvernight": False}
    if t == "" or t == "null":
        return c
    if _any(t, ["morning", "manha", "manhã", "cedo", "early"]):
        c["preference"] = "morning"
    elif _any(t, ["afternoon", "tarde", "midday"]):
        c["preference"] = "afternoon"
    elif _any(t, ["evening", "noite", "fim do dia"]):
        c["preference"] = "evening"
    elif _any(t, ["night", "madrugada", "redeye", "late night"]):
        c["preference"] = "night"
    if _any(t, ["arrive", "chegar", "chegada"]):
        c["arriveBy"] = _first_time(t)
    if c["arriveBy"] == "" and _any(t, ["depart", "sair", "saida", "partir", "after"]):
        c["departAfter"] = _first_time(t)
    if _any(t, ["no overnight", "sem pernoite", "nao pernoitar", "não pernoitar"]):
        c["noOvernight"] = True
    return c

def _matches_pref(hour, pref):
    if pref == "morning":
        return hour >= 6 and hour < 12
    if pref == "afternoon":
        return hour >= 12 and hour < 18
    if pref == "evening":
        return hour >= 18 and hour < 24
    if pref == "night":
        return hour >= 0 and hour < 6
    return True

# --- offers ------------------------------------------------------------------

def _get(d, *keys):
    for k in keys:
        if type(d) != "dict":
            return None
        d = d.get(k)
    return d

def _sum(xs):
    total = 0
    for x in xs:
        total += x
    return total

def _hour(iso):
    return int(iso.split("T")[1].split(":")[0])

def _hhmm(iso):
    return ":".join(iso.split("T")[1].split(":")[0:2])

def _flight(o):
    segs = []
    for s in o["segments"]:
        first = s["legs"][0]
        segs.append({
            "from": _get(s, "departureAirport", "code"),
            "to": _get(s, "arrivalAirport", "code"),
            "departTime": s["departureTime"],
            "arriveTime": s["arrivalTime"],
            "duration": s["totalTime"],
            "stops": len(s["legs"]) - 1,
            "connectionAirports": [_get(l, "departureAirport", "code") for l in s["legs"][1:]],
            "airline": first["carriersData"][0].get("name"),
            "airlineCode": first["carriersData"][0].get("code"),
            "flightNumber": _get(first, "flightInfo", "flightNumber"),
        })
    fare = o.get("brandedFareInfo") or {}
    features = fare.get("features") or []
    connections = sorted({_get(l, "departureAirport", "code"): True for s in o["segments"] for l in s["legs"][1:]}.keys())
    airlines = sorted({l["carriersData"][0]["code"]: True for s in o["segments"] for l in s["legs"]}.keys())
    details = sorted({f["label"]: True for f in features if f.get("category") == "BAGGAGE" and f.get("availability") == "INCLUDED"}.keys())
    return {
        "token": o.get("token"),
        "tripType": o.get("tripType"),
        "price": _get(o, "priceBreakdown", "total", "units"),
        "currency": _get(o, "priceBreakdown", "total", "currencyCode"),
        "segments": segs,
        "totalDuration": _sum([s["totalTime"] for s in o["segments"]]),
        "totalStops": _sum([len(s["legs"]) - 1 for s in o["segments"]]),
        "allConnectionAirports": connections,
        "airlines": airlines,
        "baggage": {
            "personalItem": any([f.get("featureName") == "PERSONAL_BAGGAGE" and f.get("availability") == "INCLUDED" for f in features]),
            "carryOn": fare.get("cabinBaggageExcluded") == False,
            "checkedBag": fare.get("checkinBaggageExcluded") == False,
            "details": details,
        },
        "fareName": fare.get("fareName"),
        "travelers": [{"type": t.get("travellerType"), "price": _get(t, "travellerPriceBreakdown", "total", "units")} for t in (o.get("travellerPrices") or [])],
    }

AIRLINE_NAMES = {
    "gol": ["G3"], "latam": ["LA", "JJ"], "azul": ["AD"], "american": ["AA"], "united": ["UA"],
    "delta": ["DL"], "air france": ["AF"], "klm": ["KL"], "lufthansa": ["LH"], "british": ["BA"],
    "tap": ["TP"], "avianca": ["AV"], "copa": ["CM"],
}

def _avoided(airlines, avoided):
    if avoided == "":
        return False
    codes = {}
    for name, cs in AIRLINE_NAMES.items():
        if name in avoided:
            for c in cs:
                codes[c] = True
    for a in airlines:
        if avoided in a.lower() or a in codes:  # the original only had the first test
            return True
    return False

def _preorder(items):
    # jq's [.items[]? | recurse(.items[]?)], without recursion (Starlark
    # forbids it): depth first, parents before children, in order.
    out, stack = [], list(reversed(items or []))
    for _ in range(100000):
        if not stack:
            break
        it = stack.pop()
        if type(it) == "dict":
            out.append(it)
            stack.extend(reversed(it.get("items") or []))
    return out

def _infant(offers, top):
    found = _preorder(_get(offers[0], "unifiedPriceBreakdown", "items")) if offers else []
    for it in found:
        if it.get("scope") == "FLIGHT_INFANT":
            return True, _get(it, "price", "units")
    if not top:
        return False, None
    infants = [t["price"] for t in top[0]["travelers"] if t["type"] == "INFANT"]
    return len(infants) > 0, (infants[0] if infants else None)

def score_flights(offers, prefs = None, time_text = ""):
    """Scores offers for a user. Returns the top 5 and a summary, in the
    shape the original returned."""
    prefs = prefs or {}
    time_value = prefs.get("time_value") or "balanced"
    avoided = (prefs.get("avoided_airlines") or "").lower()
    never_middle = prefs.get("never_middle_seat") in (True, 1, "true", "1")
    program = (prefs.get("loyalty_program") or "").lower()
    codes, preferred = loyalty(program)
    tc = time_constraint(time_text or "")
    pref = tc["preference"]

    scored = []
    for o in offers:
        f = _flight(o)
        s0 = f["segments"][0]
        f["departHour"] = _hour(s0["departTime"])
        f["arriveHour"] = _hour(s0["arriveTime"])
        f["departTimeStr"] = _hhmm(s0["departTime"])
        f["arriveTimeStr"] = _hhmm(s0["arriveTime"])
        f["isOvernight"] = s0["departTime"].split("T")[0] != s0["arriveTime"].split("T")[0]
        f["matchesTimePreference"] = _matches_pref(f["departHour"], pref)
        ok = True
        if tc["arriveBy"] != "":
            ok = ok and _minutes(f["arriveTimeStr"]) <= _minutes(tc["arriveBy"])
        if tc["departAfter"] != "":
            ok = ok and _minutes(f["departTimeStr"]) >= _minutes(tc["departAfter"])
        if tc["noOvernight"]:
            ok = ok and not f["isOvernight"]
        f["meetsTimeConstraints"] = ok
        f["isLoyaltyAirline"] = any([a in codes for a in f["airlines"]])
        f["isPreferredAirline"] = any([a in preferred for a in f["airlines"]])
        if time_value == "price":
            score = 100 - f["price"] / 100
        elif time_value == "time":
            score = 100 - f["totalDuration"] / 600
        else:
            score = 100 - f["price"] / 200 - f["totalDuration"] / 1200 - f["totalStops"] * 10
        if f["isPreferredAirline"]:
            score += 25
        elif f["isLoyaltyAirline"]:
            score += 15
        if f["matchesTimePreference"] and pref != "":
            score += 10
        if f["isOvernight"] and not tc["noOvernight"]:
            score -= 5
        f["score"] = score
        if ok and not _avoided(f["airlines"], avoided):
            scored.append(f)

    scored = sorted(scored, key = lambda f: -f["score"])
    top = scored[:5]
    has_infants, infant_price = _infant(offers, top)
    constraint = {}
    if pref != "" or tc["arriveBy"] != "" or tc["departAfter"] != "":
        constraint = tc
    return {
        "topFlights": top,
        "summary": {
            "total": len(offers),
            "analyzed": len(scored),
            "filtered": len(offers) - len(scored),
            "cheapestPrice": min([f["price"] for f in scored]) if scored else 0,
            "fastestDuration": min([f["totalDuration"] for f in scored]) if scored else 0,
            "currency": (scored[0]["currency"] or "BRL") if scored else "BRL",
            "loyaltyFlightsInTop5": len([f for f in top if f["isLoyaltyAirline"]]),
            "timePreferenceMatchInTop5": len([f for f in top if f["matchesTimePreference"]]),
            "baggageSummary": {
                "withCheckedBag": len([f for f in top if f["baggage"]["checkedBag"]]),
                "withCarryOn": len([f for f in top if f["baggage"]["carryOn"]]),
                "withPersonalItem": len([f for f in top if f["baggage"]["personalItem"]]),
                "allBaggageDetails": [{"fareName": f["fareName"], "baggage": f["baggage"]} for f in top],
            },
            "hasInfants": has_infants,
            "infantPrice": infant_price,
        },
        "neverMiddleSeat": never_middle,
        "loyaltyProgram": program,
        "loyaltyAirlines": ",".join(codes),
        "timeConstraint": constraint,
    }
