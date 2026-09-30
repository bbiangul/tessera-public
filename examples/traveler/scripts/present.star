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
        out.append({"label": label, "token": f["token"], "airline": s["airline"], "price": f["price"], "currency": currency, "baggage": _baggage(f), "departTime": s["departTime"],
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

# What the agency says about a trip (RFC-0009): every answer starts with
# what it assumed (the route, the dates, who travels), shows a round trip
# as both legs, gives the group's total and the price per person, and
# offers the next step. The values come from the searches; nothing here
# invents a price.

def money(v):
    s = str(int(v + 0.5))
    groups = []
    for _ in range(10):
        if len(s) <= 3:
            break
        groups.insert(0, s[-3:])
        s = s[:-3]
    groups.insert(0, s)
    return "R$ " + ".".join(groups)

def ddmm(d):
    return d[8:10] + "/" + d[5:7]

def _day(d):
    return time.parse_time(d[0:10] + "T12:00:00Z")

def nights(check_in, check_out):
    return int((_day(check_out) - _day(check_in)).hours // 24)

def party(adults, children):
    """Who travels, in words: "2 adultos e 1 criança (4 anos)"."""
    adults = adults or 1
    children = children or []
    parts = ["%d adulto%s" % (adults, "" if adults == 1 else "s")]
    if children:
        ages = [("bebê" if a < 1 else "%d ano%s" % (a, "" if a == 1 else "s")) for a in children]
        parts.append("%d criança%s (%s)" % (len(children), "" if len(children) == 1 else "s", ", ".join(ages)))
    return " e ".join(parts)

def travelers(adults, children):
    return (adults or 1) + len(children or [])

def _stops(n):
    return "direto" if n == 0 else ("%d parada" % n + ("s" if n > 1 else ""))

def _hhmm(iso):
    return iso[11:16]

def _leg(s):
    next_day = " (+1 dia)" if s["arriveTime"][0:10] != s["departTime"][0:10] else ""
    return "%s %s %s–%s%s" % (ddmm(s["departTime"]), s["from"] + "→" + s["to"], _hhmm(s["departTime"]), _hhmm(s["arriveTime"]), next_day)

def flight_line(f, adults, children):
    """One flight as a line: the airline, each leg, stops, the group's
    total and the price per person."""
    segs = f["segments"]
    legs = "ida " + _leg(segs[0])
    if len(segs) > 1:
        legs += ", volta " + _leg(segs[1])
    n = travelers(adults, children)
    price = money(f["price"]) + (" no total (%s por pessoa)" % money(f["price"] / n) if n > 1 else "")
    bag = "mala despachada inclusa" if f["baggage"]["checkedBag"] else "sem mala despachada"
    return "%s: %s (%s) — %s, %s" % (segs[0]["airline"], legs, _stops(f["totalStops"]), price, bag)

def _prices(previous, key):
    """The prices of what was shown before: a flight search's results or
    a list of stays."""
    if not previous:
        return []
    items = previous.get(key) or []
    return [it["price"] for it in items if it.get("price")]

# Periods of the day in the user's words, as (from hour, to hour).
_PERIODS = [("madrugada", 0, 5), ("cedo", 5, 10), ("manh", 5, 12), ("tarde", 12, 18), ("noite", 18, 24), ("de dia", 6, 18), ("dia", 6, 18)]

def _period(text):
    t = (text or "").lower()
    for word, a, b in _PERIODS:
        if word in t:
            return (a, b)
    return None

def _return_hour(f):
    segs = f["segments"]
    return int(segs[-1]["departTime"][11:13]) if len(segs) > 1 else None

def flights_view(ranked, origin, destination, depart, ret, adults, children, direct_only = False, reused = False, max_flights = 3, cheap = False, previous = None, other = "", return_time = ""):
    """The flights answer and what is remembered of it (the labels the
    follow-ups and the booking read). Asked for cheaper, the flights go
    from the cheapest up."""
    kept = [f for f in ranked["topFlights"] if not direct_only or f["totalStops"] == 0]
    if cheap:
        kept = sorted(kept, key = lambda f: f["price"])
    wish = _period(return_time) if ret else None
    if wish:
        # The return in the wished period first, then the rest, each in
        # the order they had.
        fits = [f for f in kept if _return_hour(f) != None and wish[0] <= _return_hour(f) and _return_hour(f) < wish[1]]
        kept = fits + [f for f in kept if f not in fits]
    top = kept[:max_flights]
    labels = labeled({"topFlights": top, "summary": ranked["summary"]}, max_flights)
    head = "✈️ %s → %s · %s · %s" % (origin, destination,
                                     ("ida %s, volta %s" % (ddmm(depart), ddmm(ret))) if ret else ("só ida, %s" % ddmm(depart)),
                                     party(adults, children))
    if reused:
        head += " (as datas e o grupo que você me passou)"
    if not top:
        msg = head + "\nNão encontrei voos%s para essas datas. Quer tentar outro dia?" % (" diretos" if direct_only else "")
        return {"message": msg, "shown": []}
    before = _prices(previous, "results")
    if cheap and before and top[0]["price"] >= min(before):
        # Asked again for cheaper, and nothing is: say so, plainly.
        top = top[:1]
        lines = [head, "Procurei de novo: o menor preço para essas datas continua sendo este, e não encontrei nada mais barato:"]
    else:
        lines = [head]
        if cheap:
            lines.append("Do mais barato para o mais caro:")
        if wish:
            fitting = len([f for f in top if _return_hour(f) != None and wish[0] <= _return_hour(f) and _return_hour(f) < wish[1]])
            lines.append("Com a volta nesse horário, primeiro:" if fitting else "Não encontrei volta nesse horário; estas são as opções que há:")
    shown = []
    for i, f in enumerate(top):
        line = "%d. %s" % (i + 1, flight_line(f, adults, children))
        lines.append(line)
        if labels[i].get("warning"):
            lines.append(labels[i]["warning"])
        item = dict(labels[i])
        item["label"] = line
        item["summary"] = flight_line(f, adults, children)
        shown.append(item)
    if ranked["summary"]["hasInfants"]:
        lines.append("Com bebê de colo: confira com a companhia a política de colo e de berço.")
    if other:
        lines.append("Quer que eu veja %s também, nas mesmas datas, para comparar?" % other)
    elif ret:
        lines.append("Quer que eu veja hospedagem em %s nessas datas, ou já reservo um desses?" % destination)
    else:
        lines.append("Quer que eu reserve um desses, ou prefere outro horário?")
    return {"message": "\n".join(lines), "shown": shown}

_KINDS = {"Hotel": "hotel", "Apartment": "apartamento", "Guest house": "pousada", "Hostel": "hostel"}

def _districts(raw):
    """The area of each hotel as the search gave it, by hotel id."""
    out = {}
    for h in raw or []:
        p = h.get("property") or {}
        d = p.get("district") or p.get("wishlistName") or ""
        if d:
            out[str(h.get("hotel_id"))] = d
    return out

def hotel_line(h, n):
    kind = _KINDS.get(h["propertyType"], h["propertyType"].lower())
    where = (" em " + h["district"]) if h.get("district") else ""
    price = ", %s a estadia (%s/noite)" % (money(h["pricePerNight"]), money(h["pricePerNight"] / max(n, 1))) if h["pricePerNight"] > 0 else ""
    cancel = ", cancelamento grátis" if h["freeCancellation"] else ""
    return "%s (%s%s, nota %s)%s%s" % (h["name"], kind, where, h["reviewScore"], price, cancel)

def with_districts(ranked, raw):
    """The ranked hotels with the area each one is in."""
    areas = _districts(raw)
    top = []
    for h in ranked["topHotels"]:
        h = dict(h)
        h["district"] = areas.get(h["hotelId"], "")
        top.append(h)
    return {"topHotels": top, "summary": ranked["summary"]}

_STAY_KINDS = {"hotel": "Hotéis", "flexible": "Hotéis e apart-hotéis", "aparthotel": "Apart-hotéis",
               "serviced_apartment": "Apartamentos com serviço", "apartment": "Apartamentos"}

def hotels_view(ranked, place, check_in, check_out, adults, children, reused = False, max_hotels = 3, raw = None, cheap = False, previous = None, other = "", kind = ""):
    n = nights(check_in, check_out)
    ranked = with_districts(ranked, raw)
    kept = ranked["topHotels"]
    if cheap:
        kept = sorted([h for h in kept if h["pricePerNight"] > 0], key = lambda h: h["pricePerNight"])
    top = kept[:max_hotels]
    head = "🏨 %s em %s · %s–%s (%d noite%s) · %s" % (_STAY_KINDS.get(kind, "Hospedagem"), place["name"], ddmm(check_in), ddmm(check_out), n, "" if n == 1 else "s", party(adults, children))
    if reused:
        head += " (as datas e o grupo da sua viagem)"
    if not top:
        return {"message": head + "\nNão encontrei hospedagem para essas datas. Quer tentar outras datas ou outra região?", "shown": []}
    before = _prices(previous, "items")
    if cheap and before and top[0]["pricePerNight"] >= min(before):
        top = top[:1]
        lines = [head, "Procurei de novo: a hospedagem mais barata para essas datas continua sendo esta, e não encontrei nada abaixo disso:"]
    else:
        lines = [head]
        if cheap:
            lines.append("Do mais barato para o mais caro:")
    shown = []
    for i, h in enumerate(top):
        line = "%d. %s" % (i + 1, hotel_line(h, n))
        lines.append(line)
        shown.append({"label": line, "summary": hotel_line(h, n), "hotelId": h["hotelId"], "name": h["name"], "price": h["pricePerNight"],
                      "nights": n, "checkIn": check_in, "checkOut": check_out, "freeCancellation": h["freeCancellation"]})
    if other:
        lines.append("Quer que eu veja a hospedagem em %s também, para comparar?" % other)
    elif cheap and len(top) == 1:
        lines.append("Quer que eu tente outras datas, ou reservo este?")
    else:
        lines.append("Quer que eu reserve um desses, ou busco algo mais barato ou em outra região?")
    return {"message": "\n".join(lines), "shown": shown}

def _pick(items, wanted, text):
    """The index of the first item whose text names what the user picked."""
    w = (wanted or "").lower().strip()
    if w == "":
        return None
    for i, it in enumerate(items):
        if w in text(it).lower() or text(it).lower() in w:
            return i
    return None

def _pick_flight(fs, wanted):
    """The flight the user names: by airline, and by a time of either leg
    when several flights of that airline were shown."""
    w = (wanted or "").lower()
    if w == "":
        return None
    by_airline = [i for i in range(len(fs)) if fs[i]["segments"][0]["airline"].lower() in w]
    if not by_airline:
        return None
    for i in by_airline:
        for seg in fs[i]["segments"]:
            hhmm = seg["departTime"][11:16]
            if hhmm in w or hhmm.replace(":", "h") in w or (hhmm.endswith(":00") and hhmm[0:2].lstrip("0") + "h" in w):
                return i
    return by_airline[0]

def quote(flights, hotels, place, origin, destination, depart, ret, adults, children, hotel_choice = "", flight_choice = ""):
    """The whole trip: the recommended flight and stay (or the ones the user
    picked by name), the total and the price per person, and the cheapest
    combination when it differs."""
    fs = flights["topFlights"]
    hs = hotels["topHotels"]
    n = travelers(adults, children)
    stay = nights(depart, ret)
    head = "🧳 Cotação %s → %s · ida %s, volta %s (%d noite%s) · %s" % (origin, destination, ddmm(depart), ddmm(ret), stay, "" if stay == 1 else "s", party(adults, children))
    if not fs or not hs:
        missing = "voos" if not fs else "hospedagem"
        return {"message": head + "\nNão encontrei %s para essas datas. Quer tentar outras datas?" % missing, "options": []}
    labels = labeled({"topFlights": fs, "summary": flights["summary"]}, len(fs))
    options = []

    def option(title, fi, hi):
        f, h = fs[fi], hs[hi]
        total = f["price"] + h["pricePerNight"]
        fl = dict(labels[fi])
        fl["label"] = flight_line(f, adults, children)
        fl["summary"] = fl["label"]
        ho = {"label": hotel_line(h, stay), "summary": hotel_line(h, stay), "hotelId": h["hotelId"], "name": h["name"],
              "price": h["pricePerNight"], "nights": stay, "checkIn": depart, "checkOut": ret, "freeCancellation": h["freeCancellation"]}
        text = "%s\n   ✈️ %s\n   🏨 %s\n   Total: %s%s" % (title, fl["summary"], ho["summary"], money(total),
                                                          " (%s por pessoa)" % money(total / n) if n > 1 else "")
        return {"label": title + ": " + fl["summary"] + " + " + ho["summary"] + ", total " + money(total), "text": text,
                "flight": fl, "hotel": ho, "total": total}

    hi = _pick(hs, hotel_choice, lambda h: h["name"])
    fi = _pick_flight(fs, flight_choice)
    chosen = hi != None or fi != None
    options.append(option("1. Com a sua escolha" if chosen else "1. Recomendada", fi or 0, hi or 0))
    if (hotel_choice and hi == None) or (flight_choice and fi == None):
        missing = hotel_choice if hotel_choice and hi == None else flight_choice
        head += "\n(Não encontrei \"%s\" nesta busca; usei a opção recomendada.)" % missing
    cheap_f = sorted(range(len(fs)), key = lambda i: fs[i]["price"])[0]
    priced = [i for i in range(len(hs)) if hs[i]["pricePerNight"] > 0]
    cheap_h = sorted(priced, key = lambda i: hs[i]["pricePerNight"])[0] if priced else 0
    if (cheap_f, cheap_h) != (fi or 0, hi or 0):
        options.append(option("2. Mais econômica", cheap_f, cheap_h))
    lines = [head] + [o["text"] for o in options]
    lines.append("Os preços são os de agora e só ficam garantidos na reserva. Quer que eu reserve uma dessas?")
    return {"message": "\n".join(lines), "options": options}

def booking_summary(flight, hotel, adults, children, passengers, bags = None):
    """What the customer confirms: every item, who travels, the total."""
    lines = []
    total = (bags or {}).get("total") or 0
    if flight:
        lines.append("✈️ " + flight["summary"])
        total += flight["price"]
        if (bags or {}).get("line"):
            lines.append(bags["line"])
    if hotel:
        lines.append("🏨 %s, %s a %s" % (hotel["summary"], ddmm(hotel["checkIn"]), ddmm(hotel["checkOut"])))
        total += hotel["price"]
    names = ", ".join([p.get("name") or "?" for p in passengers])
    lines.append("Viajantes: " + names)
    lines.append("Total: " + money(total))
    depart = flight["departTime"][0:10] if flight else hotel["checkIn"]
    return {"text": "\n".join(lines), "total": total, "totalText": money(total), "names": names, "departDate": depart}

def missing_passengers(passengers, expected):
    """How many travelers still lack a name or a birth date."""
    ok = [p for p in (passengers or []) if p.get("name") and p.get("birthDate")]
    return max(expected - len(ok), 0)

def _short(o):
    """An option in a few words: the airline and time, and the stay."""
    f, h = o["flight"], o["hotel"]
    return "%s %s + %s" % (f.get("airline") or "", (f.get("departTime") or "")[11:16], h["name"])

def compare(quotes, origin, depart, ret, adults, children):
    """Two whole trips side by side, and how much the cheaper one saves."""
    n = travelers(adults, children)
    lines = ["⚖️ Comparação saindo de %s · ida %s, volta %s · %s" % (origin, ddmm(depart), ddmm(ret), party(adults, children))]
    cheapest = []
    for q in quotes:
        opts = q["options"]
        if not opts:
            lines.append("• %s: não encontrei voo e hospedagem para essas datas." % q["destination"])
            continue
        low = sorted(opts, key = lambda o: o["total"])[0]
        best = opts[0]
        line = "• %s: a partir de %s no total (%s)" % (q["destination"], money(low["total"]), _short(low))
        if best["total"] != low["total"]:
            line += "; recomendado %s (%s)" % (money(best["total"]), _short(best))
        lines.append(line)
        cheapest.append((low["total"], q["destination"]))
    if len(cheapest) == 2:
        (a, da), (b, db) = sorted(cheapest)
        if a == b:
            lines.append("Pelo mais econômico, os dois saem pelo mesmo preço.")
        else:
            diff = b - a
            per = (" (%s por pessoa)" % money(diff / n)) if n > 1 else ""
            lines.append("Pelo mais econômico, %s sai %s mais barato que %s%s." % (da, money(diff), db, per))
    lines.append("Quer ver os detalhes de um deles, ou já reservo o pacote de um dos dois?")
    return "\n".join(lines)

_STATES = {
    "requested": "pedido registrado; a equipe está confirmando a disponibilidade e manda o localizador e o link de pagamento por aqui",
    "reserved": "reservada, aguardando o pagamento; a emissão sai quando o pagamento for aprovado",
    "paid": "paga; a equipe está emitindo os bilhetes",
    "issued": "paga e emitida ✅",
}

def booking_status(booking, no_email = False):
    """A booking's state, as the agency's system has it."""
    b = booking
    lines = []
    head = "Sua reserva"
    if b.get("locator"):
        head += " %s" % b["locator"]
    lines.append("%s: %s." % (head, _STATES.get(b.get("status"), b.get("status") or "sem status")))
    lines.append(b.get("summary") or "")
    if b.get("status") == "reserved" and b.get("payment"):
        lines.append("Link de pagamento: %s" % b["payment"])
    if b.get("eticket"):
        lines.append("E-ticket: %s" % b["eticket"])
    if no_email:
        lines.append("Pedi para a equipe reenviar o e-mail com o bilhete; ele chega em breve (confira também a caixa de spam).")
    return "\n".join([l for l in lines if l])
