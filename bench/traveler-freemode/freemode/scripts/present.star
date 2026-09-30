# The message with the best flights, in Portuguese. Built here and not in a
# template because it is a list with optional parts; the values come from
# score_flights and the text is escaped by the runtime.

def _money(v, currency):
    whole = int(v)
    s = str(whole)
    groups = []
    for _ in range(10):
        if len(s) <= 3:
            break
        groups.insert(0, s[-3:])
        s = s[:-3]
    groups.insert(0, s)
    prefix = "R$ " if currency == "BRL" else currency + " "
    return prefix + ".".join(groups)

def _duration(seconds):
    h = seconds // 3600
    m = (seconds % 3600) // 60
    return "%dh%s" % (h, ("0" + str(m))[-2:])

def _baggage(f):
    b = f["baggage"]
    parts = []
    if b["personalItem"]:
        parts.append("item pessoal")
    if b["carryOn"]:
        parts.append("bagagem de mão")
    have = ", ".join(parts) if parts else "nenhuma bagagem"
    checked = "mala despachada inclusa" if b["checkedBag"] else "mala despachada não inclusa"
    fare = ("Tarifa %s: " % f["fareName"]) if f.get("fareName") else ""
    return "%s%s; %s." % (fare, have, checked)

def labeled(ranked, max_flights = 3):
    """The flights as the user saw them: one label per flight (the line of
    the message) plus what follow-up questions need."""
    currency = ranked["summary"]["currency"]
    out = []
    for i, f in enumerate(ranked["topFlights"][:max_flights]):
        s = f["segments"][0]
        stops = "direto" if f["totalStops"] == 0 else ("%d parada" % f["totalStops"] + ("s" if f["totalStops"] > 1 else ""))
        bag = ", mala despachada inclusa" if f["baggage"]["checkedBag"] else ""
        label = "%d. %s %s → %s, %s %s–%s (%s, %s) — %s%s" % (
            i + 1, s["airline"], s["from"], s["to"], s["departTime"][8:10] + "/" + s["departTime"][5:7],
            f["departTimeStr"], f["arriveTimeStr"], stops, _duration(f["totalDuration"]),
            _money(f["price"], currency), bag)
        out.append({"label": label, "token": f["token"], "airline": s["airline"], "price": f["price"], "currency": currency, "baggage": _baggage(f),
                    "warning": _change_warning(f)})
    return out

def _change_warning(f):
    """A connection that changes airports, as the original told it: a
    strong warning when the layover is too short for the drive, a caution
    when it only works without checked bags, a note otherwise."""
    changes = f.get("airportChanges") or []
    if not changes:
        return ""
    parts = []
    for c in changes:
        parts.append("%s→%s: conexão de %dmin, translado ~%dmin" % (c["from"], c["to"], c["layover"], c["transfer"]))
    what = "troca de aeroporto (" + "; ".join(parts) + ")"
    risk = f.get("airportChangeRisk")
    if risk == "HIGH":
        return "   ⛔ Exige " + what + ". Não recomendado: alto risco de perder a conexão."
    if risk == "MEDIUM":
        return "   ⚠️ Exige " + what + ". Apertado: só viável sem mala despachada."
    return "   ℹ️ Exige " + what + ", com tempo suficiente."

def flights_message(ranked, max_flights = 3):
    flights = labeled(ranked, max_flights)
    if not flights:
        return "Não encontrei voos para essa busca. Quer tentar outra data?"
    lines = ["Encontrei estas opções:"]
    for f in flights:
        lines.append(f["label"])
        if f.get("warning"):
            lines.append(f["warning"])
    if ranked["summary"]["hasInfants"]:
        lines.append("Há bebê na reserva: confira a política da companhia para colo e berço.")
    return "\n".join(lines)
