# Hotel scoring for the traveler agent: a port of the legacy platform's scoreHotels
# (bash + jq), compared with the original on the same fixtures in
# scoring_test.go. Intended differences (bugs fixed):
#
#   1. serviced_apartment, the type the original chose for stays over 14
#      nights, gets the apartment bonus; the original's scoring never
#      matched it.
#   2. A hotel without a price gets no price bonus; the original read the
#      missing price as 0 and ranked it as the cheapest.
#   3. The API's grossPrice is the whole stay, but the price thresholds are
#      per night; given the nights, the price is divided before comparing.
#      Without it (the parity fixtures) the original's comparison stays.

def _num(v):
    return v if type(v) in ("int", "float") else 0

def _type_bonus(wanted, actual):
    actual = (actual or "hotel").lower()
    if wanted == "hotel" and "hotel" in actual:
        return 15
    if wanted in ("aparthotel", "apartment", "serviced_apartment") and "apart" in actual:
        return 15
    return 0

def score_hotels(hotels, prefs = None, property_type = "hotel", nights = 1):
    prefs = prefs or {}
    time_value = prefs.get("time_value") or "balanced"
    wanted = property_type or "hotel"
    scored = []
    for h in hotels:
        p = h.get("property") or {}
        gross = (p.get("priceBreakdown") or {}).get("grossPrice") or {}
        review = _num(p.get("reviewScore"))
        price = gross.get("value")
        photos = p.get("photoUrls") or []
        out = {
            "hotelId": str(h.get("hotel_id")),
            "name": p.get("name"),
            "reviewScore": review,
            "reviewCount": p.get("reviewCount") or 0,
            "pricePerNight": _num(price),
            "currency": gross.get("currency") or "BRL",
            "propertyType": p.get("propertyType") or "hotel",
            "freeCancellation": p.get("freeCancellation") or False,
            "photoUrl": photos[0] if photos else None,
            "distanceToCenter": (p.get("distanceToCenter") or {}).get("label") or "unknown",
            "checkinTime": p.get("checkinDate"),
            "checkoutTime": p.get("checkoutDate"),
            "score": 100,
            "excluded": False,
            "exclusionReason": None,
        }
        adjusted = 100
        if review >= 8.5:
            adjusted += 20
        elif review >= 8.0:
            adjusted += 10
        elif review < 7.5:
            adjusted -= 20
        if out["freeCancellation"]:
            adjusted += 10
        if wanted != "flexible":
            adjusted += _type_bonus(wanted, out["propertyType"])
        if time_value == "price" and type(price) in ("int", "float"):
            nightly = price / max(nights, 1) if nights > 1 else price
            if nightly < 200:
                adjusted += 15
            elif nightly < 400:
                adjusted += 5
            else:
                adjusted -= 10
        out["adjustedScore"] = adjusted
        if review >= 7.0:
            scored.append(out)
    top = sorted(scored, key = lambda h: -h["adjustedScore"])[:10]
    return {
        "topHotels": top,
        "summary": {"total": len(hotels), "scored": len(top), "excluded": len(hotels) - len(top), "propertyType": wanted},
    }

def pick_destination(places):
    """The original's resolveDestination: a single result wins; otherwise
    the first city (more hotels than a district or a landmark); otherwise
    the first result. Fails with NO_DESTINATION."""
    if not places:
        fail("NO_DESTINATION")
    chosen = places[0]
    if len(places) > 1:
        for p in places:
            if p.get("dest_type") == "city":
                chosen = p
                break
    return {"destId": str(chosen["dest_id"]), "searchType": chosen.get("dest_type") or "city",
            "name": chosen.get("city_name") or chosen.get("name")}

def _day(d):
    return time.parse_time(d + "T12:00:00Z")

def nights(check_in, check_out):
    return int((_day(check_out) - _day(check_in)).hours // 24)

def property_type(check_in, check_out, wanted = ""):
    """The original's rule: what the user asked for; otherwise by the length
    of the stay (up to 3 nights a hotel, up to 7 either, up to 14 an
    aparthotel, longer a serviced apartment)."""
    n = nights(check_in, check_out)
    if wanted in ("hotel", "apartment"):
        return {"kind": wanted, "nights": n, "asked": True}
    if n <= 3:
        kind = "hotel"
    elif n <= 7:
        kind = "flexible"
    elif n <= 14:
        kind = "aparthotel"
    else:
        kind = "serviced_apartment"
    return {"kind": kind, "nights": n, "asked": False}

_KINDS = {"hotel": "hotéis", "flexible": "hotéis e apart-hotéis", "aparthotel": "apart-hotéis",
          "serviced_apartment": "apartamentos com serviço", "apartment": "apartamentos"}

def _money(v):
    s = str(int(v))
    groups = []
    for _ in range(10):
        if len(s) <= 3:
            break
        groups.insert(0, s[-3:])
        s = s[:-3]
    groups.insert(0, s)
    return ".".join(groups)

def _ddmm(d):
    return d[8:10] + "/" + d[5:7]

def hotels_message(ranked, place, stay, check_in, check_out, max_hotels = 3):
    top = ranked["topHotels"][:max_hotels]
    if not top:
        return "Não encontrei hospedagem em %s para essas datas. Quer tentar outras datas?" % place["name"]
    n = stay["nights"]
    head = "%s em %s, %s–%s (%d noite%s):" % (_KINDS.get(stay["kind"], "hospedagens").capitalize(), place["name"],
                                              _ddmm(check_in), _ddmm(check_out), n, "" if n == 1 else "s")
    lines = [head]
    for i, h in enumerate(top):
        currency = "R$ " if h["currency"] == "BRL" else h["currency"] + " "
        price = ""
        if h["pricePerNight"] > 0:  # the API's grossPrice: the whole stay
            price = ", %s%s no total (%s%s/noite)" % (currency, _money(h["pricePerNight"]), currency, _money(h["pricePerNight"] / max(n, 1)))
        cancel = ", cancelamento grátis" if h["freeCancellation"] else ""
        lines.append("%d. %s — nota %s (%d avaliações)%s%s" % (i + 1, h["name"], h["reviewScore"], h["reviewCount"], price, cancel))
    return "\n".join(lines)
