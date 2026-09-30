# Seat availability for the traveler's top flights: a port of the legacy platform's
# checkTopFlightsSeatAvailability (bash + jq), compared with the original in
# scoring_test.go. Intended difference: a bassinet seat is counted once;
# the original counted a row once per bassinet seat in it.

def _hour(iso):
    t = iso.split("T")
    if len(t) < 2 or len(t[1]) < 2 or not t[1][:2].isdigit():
        return -1
    return int(t[1][:2])

def _flags(f):
    long = (f.get("totalDuration") or 0) // 3600 >= 4
    segs = f.get("segments") or []
    hour = _hour((segs[0].get("departTime") or "") if segs else "")
    overnight = hour >= 0 and (hour >= 21 or hour <= 5 or (long and hour >= 18))
    return long, overnight

def _analyze(seat_map):
    m = (seat_map.get("data") or {}).get("seatMap") or {}
    col_types = {c["id"]: c.get("type") for c in (m.get("columns") or [])}
    rows = m.get("rows") or []
    normal = [s for r in rows for s in (r.get("seats") or []) if s.get("description") == "NORMAL"]
    non_middle = [s for s in normal if col_types.get(s.get("colId")) != "BETWEEN"]
    free = [s for s in normal if s.get("priceBreakdown") == None]
    free_non_middle = [s for s in non_middle if s.get("priceBreakdown") == None]
    paid = [s for s in non_middle if s.get("priceBreakdown") != None]
    prices = [s["priceBreakdown"]["total"]["units"] for s in paid]
    currency = paid[0]["priceBreakdown"]["total"].get("currencyCode") if paid else "BRL"
    if free_non_middle:
        min_price = 0
    elif prices:
        min_price = min(prices)
    else:
        min_price = None

    def bassinet_row(r):
        if (r.get("rowNumber") or 0) <= 2:
            return True
        for s in r.get("seats") or []:
            for c in s.get("seatCharacteristic") or []:
                if "bassinet" in c.lower():
                    return True
        return False

    bassinet = [s for r in rows if bassinet_row(r) for s in (r.get("seats") or []) if s.get("description") == "NORMAL"]
    bassinet_free = [s for s in bassinet if s.get("priceBreakdown") == None]
    bassinet_prices = [s["priceBreakdown"]["total"]["units"] for s in bassinet if s.get("priceBreakdown") != None]
    if not bassinet:
        bassinet_price = None
    elif bassinet_free:
        bassinet_price = 0
    elif bassinet_prices:
        bassinet_price = min(bassinet_prices)
    else:
        bassinet_price = None
    return {
        "onlyMiddle": len(non_middle) == 0 and len(normal) > 0,
        "free": len(free),
        "freeNonMiddle": len(free_non_middle) > 0,
        "minPrice": min_price,
        "currency": currency or "BRL",
        "bassinet": len(bassinet),
        "bassinetPrice": bassinet_price,
    }

def _price_text(p):
    # jq -r prints 45 and 39.9; keep the same text in the warning.
    if type(p) == "float" and p == int(p):
        return str(int(p))
    return str(p)

def analyze_seats(scored, seat_maps):
    """One entry per top flight with a token, in order; seat_maps[i] is the
    map for topFlights[i]."""
    out = []
    infants = ((scored.get("summary") or {}).get("hasInfants")) == True
    for i, f in enumerate(scored.get("topFlights") or []):
        token = f.get("token")
        if token == None or token == "":
            continue
        long, overnight = _flags(f)
        sm = seat_maps[i] if i < len(seat_maps) else None
        if sm == None or sm.get("status") != True:
            out.append({"token": token, "seatStatus": "unknown", "onlyMiddleSeats": False, "freeSeatsCount": 0,
                        "isLongFlight": long, "isOvernight": overnight, "warning": None})
            continue
        a = _analyze(sm)
        status, warning = "ok", None
        if a["onlyMiddle"]:
            status = "middle_only"
            warning = "Only middle seats available on long flight" if long else "Only middle seats available"
        elif not a["freeNonMiddle"] and a["minPrice"] != None and a["minPrice"] != 0:
            status = "paid_only"
            warning = "Window/aisle seats from %s %s" % (a["currency"], _price_text(a["minPrice"]))
        out.append({
            "token": token, "seatStatus": status, "onlyMiddleSeats": a["onlyMiddle"], "freeSeatsCount": a["free"],
            "isLongFlight": long, "isOvernight": overnight, "warning": warning,
            "windowAisleSeatPrice": a["minPrice"], "seatCurrency": a["currency"],
            "hasBassinetSeats": a["bassinet"] > 0, "bassinetSeatsCount": a["bassinet"],
            "bassinetSeatPrice": a["bassinetPrice"], "infantsOnFlight": infants,
        })
    return out
