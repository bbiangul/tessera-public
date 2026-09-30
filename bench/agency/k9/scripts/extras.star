# What can be bought for a flight already shown: checked bags, the flexible
# ticket and travel insurance, from Booking.com's getSeatMap. the legacy platform
# asked a model to read the whole response and write an analysis; here the
# numbers are read by code and the message is built from them, so a price
# is never invented.

def _value(p):
    total = (p or {}).get("total") or {}
    units = total.get("units")
    if units == None:
        return None
    return units + (total.get("nanos") or 0) / 1e9

def _money(p):
    total = (p or {}).get("total") or {}
    value = _value(p)
    if value == None:
        return None
    return _fmt(value, total.get("currencyCode") or "BRL")

def _fmt(value, currency):
    whole = int(value + 0.5)
    s = str(whole)
    groups = []
    for _ in range(10):
        if len(s) <= 3:
            break
        groups.insert(0, s[-3:])
        s = s[:-3]
    groups.insert(0, s)
    return ("R$ " if currency == "BRL" else currency + " ") + ".".join(groups)

def _kg(allowance):
    w = allowance.get("maxWeightPerPiece")
    if w == None:
        return ""
    if allowance.get("massUnit") == "LB":
        w = w * 0.4536
    return " de até %d kg" % int(w + 0.5)

def extras(data):
    """The extras on sale, as data: bags, flexible ticket, insurance."""
    data = data or {}
    bags = []
    for o in (data.get("checkedInBaggage") or {}).get("options") or []:
        a = o.get("luggageAllowance") or {}
        price = _money(o.get("priceBreakdown"))
        if price:
            bags.append({"pieces": a.get("maxPiece") or 1, "weight": _kg(a), "price": price, "value": _value(o.get("priceBreakdown")),
                         "currency": ((o.get("priceBreakdown") or {}).get("total") or {}).get("currencyCode") or "BRL"})
    flex = _money((data.get("flexibleTicket") or {}).get("priceBreakdown"))
    ins = (data.get("travelInsurance") or {})
    insurance = _money(((ins.get("options") or {}).get("priceBreakdown")))
    benefits = ((ins.get("content") or {}).get("benefits") or [])[:3]
    return {"bags": bags, "flexibleTicket": flex, "insurance": insurance, "insuranceBenefits": benefits}

def extras_message(flight, data, travelers = 1):
    x = extras(data)
    lines = [flight["label"]]
    if flight.get("baggage"):
        lines.append("Incluso: " + flight["baggage"])
    for b in x["bags"]:
        lines.append("Mala despachada (%d peça%s%s): %s" % (b["pieces"], "" if b["pieces"] == 1 else "s", b["weight"], b["price"]))
        if travelers > 1:
            lines.append("   Uma mala para cada um dos %d viajantes: %s" % (travelers, _fmt(b["value"] * travelers, b["currency"])))
    if x["flexibleTicket"]:
        lines.append("Bilhete flexível (remarcar ou cancelar): %s" % x["flexibleTicket"])
    if x["insurance"]:
        extra = (" — cobre " + "; ".join(x["insuranceBenefits"]).lower()) if x["insuranceBenefits"] else ""
        lines.append("Seguro viagem: %s%s" % (x["insurance"], extra))
    if len(lines) == (2 if flight.get("baggage") else 1):
        lines.append("A companhia não oferece extras para compra nesse voo.")
    return "\n".join(lines)

def bag_for(data, wanted, travelers = 1):
    """One checked bag for each traveler, added to a booking: its price
    from the seat map, or the team to price it when the map had none."""
    if not wanted:
        return {"line": "", "total": 0, "count": 0}
    x = extras(data)
    n = max(travelers, 1)
    if not x["bags"]:
        return {"line": "🧳 Mala despachada: a equipe confirma o preço na reserva", "total": 0, "count": n}
    b = x["bags"][0]
    total = b["value"] * n
    each = "" if n == 1 else " (%d malas, %s cada)" % (n, b["price"])
    return {"line": "🧳 Mala despachada%s: %s" % (each, _fmt(total, b["currency"])), "total": int(total + 0.5), "count": n}

def bag_offer(data, travelers = 1):
    """A line offering a checked bag at its price, when the fare has none."""
    if not data:
        return ""
    x = extras(data)
    if not x["bags"]:
        return ""
    b = x["bags"][0]
    n = max(travelers, 1)
    each = "" if n == 1 else " cada (%s para os %d)" % (_fmt(b["value"] * n, b["currency"]), n)
    return "(Sem mala despachada. Dá para incluir por %s%s: é só responder com mala.)" % (b["price"], each)
