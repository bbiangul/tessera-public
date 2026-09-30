# Rescheduling a flight leg of a booking (agency-008). The planner did the
# arithmetic and picked flights, and got them wrong: a round trip priced for
# one leg, another airline, no total. Code reads the booking, picks the
# same airline on the new date and computes new fare - fare paid + the
# airline's fee, from the agency's rules (docs/agencia.md).

FEES = {"GOL": 150, "LATAM": 180, "Azul": 160}  # per leg and passenger; none with the flexible ticket

_DAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]

def _leap(y):
    return (y % 4 == 0 and y % 100 != 0) or y % 400 == 0

def _month_days(y, m):
    return 29 if m == 2 and _leap(y) else _DAYS[m - 1]

def _two(n):
    return ("0" + str(n))[-2:]

def _shift(day, n):
    y, m, d = [int(x) for x in day.split("-")]
    for _ in range(abs(n)):
        if n > 0:
            d += 1
            if d > _month_days(y, m):
                d, m = 1, m + 1
                if m > 12:
                    m, y = 1, y + 1
        else:
            d -= 1
            if d < 1:
                m -= 1
                if m < 1:
                    m, y = 12, y - 1
                d = _month_days(y, m)
    return "%d-%s-%s" % (y, _two(m), _two(d))

def _after(text, word):
    i = text.find(word)
    if i < 0:
        return None
    return text[i + len(word):]

def _date(ddmm, year):
    parts = ddmm.split("/")
    if len(parts) != 2 or not parts[0].isdigit() or not parts[1].isdigit():
        return None
    return "%d-%s-%s" % (year, _two(int(parts[1])), _two(int(parts[0])))

def _money(v):
    s = str(int(v))
    groups = []
    for _ in range(10):
        if len(s) <= 3:
            break
        groups.insert(0, s[-3:])
        s = s[:-3]
    groups.insert(0, s)
    return "R$ " + ".".join(groups)

def _br(day):
    return day[8:10] + "/" + day[5:7]

def _route(text):
    """'GRU→REC 12:40–17:00' → ('GRU', 'REC', '12:40'); None when absent."""
    words = text.strip().split(" ")
    if not words or "→" not in words[0]:
        return None
    frm, to = words[0].split("→")[0], words[0].split("→")[1].rstrip(",")
    time = words[1][:5] if len(words) > 1 and ":" in words[1] else ""
    return (frm, to, time)

def plan(booking, leg, new_date, shift, today):
    """What is changed: {ok, message} on a problem, else the leg, the
    dates, the airline, the passengers, the fare paid for the leg and the
    fee."""
    summary = booking.get("summary") or ""
    airline = (_after(summary, "✈️ ") or "").split(":")[0].strip()
    ida = _after(summary, "ida ")
    if airline not in FEES or ida == None or not booking.get("departDate"):
        return {"ok": False, "message": "Não consegui ler os voos dessa reserva por aqui. Vou pedir para a equipe ver a remarcação."}
    year = int(booking["departDate"][0:4])
    out_date = _date(ida.split(" ")[0], year)
    out_route = _route(ida[len(ida.split(" ")[0]):])
    back = _after(summary, "volta ")
    back_date, back_route = None, None
    if back != None:
        back_date = _date(back.split(" ")[0], year)
        if back_date != None and back_date < out_date:
            back_date = _date(back.split(" ")[0], year + 1)
        back_route = _route(back[len(back.split(" ")[0]):])
    if leg == "volta" and back_date == None:
        return {"ok": False, "message": "Essa reserva é só de ida: não há volta para remarcar."}
    if out_route == None:
        return {"ok": False, "message": "Não consegui ler os voos dessa reserva por aqui. Vou pedir para a equipe ver a remarcação."}
    if leg == "volta":
        frm, to = out_route[1], out_route[0]
        time = back_route[2] if back_route != None else ""
        current = back_date
    else:
        frm, to, time = out_route
        current = out_date
    day = new_date if new_date and len(new_date) == 10 else (_shift(current, shift) if shift != None else None)
    if day == None:
        return {"ok": False, "message": "Para qual dia você quer mudar a " + leg + "?"}
    if day < today:
        return {"ok": False, "message": "Entendi a nova data como " + _br(day) + ", que já passou. Para qual dia você quer mudar a " + leg + "?"}
    if day == current:
        return {"ok": False, "message": "A " + leg + " já está marcada para " + _br(day) + ". Para qual dia você quer mudar?"}
    if leg == "ida" and back_date != None and day > back_date:
        return {"ok": False, "message": "A nova ida ficaria depois da volta (" + _br(back_date) + "). Quer mudar as duas?"}
    if leg == "volta" and day < out_date:
        return {"ok": False, "message": "A nova volta ficaria antes da ida (" + _br(out_date) + "). Para qual dia?"}
    adults = 1
    i = summary.find(" adulto")
    if i > 0:
        n = summary[:i].split(" ")[-1]
        if n.isdigit():
            adults = int(n)
    flexible = "flexível" in summary
    paid = None
    if "🏨" not in summary:
        paid = booking.get("total") // 2 if back_date != None else booking.get("total")
    return {"ok": True, "leg": leg, "from": frm, "to": to, "time": time, "current": current, "newDate": day,
            "airline": airline, "adults": adults, "paid": paid, "fee": 0 if flexible else FEES[airline] * adults,
            "flexible": flexible}

def quote(p, offers, locator):
    """The same airline on the new date, at the booked time when there is
    one, and the estimate. {ok, message, flight, total}."""
    same = []
    for o in offers or []:
        seg = (o.get("segments") or [{}])[0]
        legs = seg.get("legs") or [{}]
        name = ((legs[0].get("carriersData") or [{}])[0]).get("name")
        if name == p["airline"]:
            same.append(o)
    if not same:
        return {"ok": False, "message": "Não achei voo da " + p["airline"] + " de " + p["from"] + " para " + p["to"] + " em " + _br(p["newDate"]) + ". Quer outro dia?"}
    pick = None
    for o in same:
        if p["time"] and o["segments"][0]["departureTime"][11:16] == p["time"]:
            pick = o
    if pick == None:
        for o in same:
            if pick == None or o["priceBreakdown"]["total"]["units"] < pick["priceBreakdown"]["total"]["units"]:
                pick = o
    fare = pick["priceBreakdown"]["total"]["units"]
    seg = pick["segments"][0]
    dep, arr = seg["departureTime"][11:16], seg["arrivalTime"][11:16]
    lines = ["Remarcar a %s da reserva %s para %s: o voo %s %s→%s das %s–%s está %s%s." % (
        p["leg"], locator, _br(p["newDate"]), p["airline"], p["from"], p["to"], dep, arr, _money(fare),
        "" if p["adults"] == 1 else " para os %d" % p["adults"])]
    total = None
    if p["paid"] != None:
        diff = fare - p["paid"]
        if diff < 0:
            diff = 0
        total = diff + p["fee"]
        fee = "sem multa (bilhete flexível)" if p["flexible"] else "a multa da %s de %s" % (p["airline"], _money(p["fee"]))
        lines.append("Você pagou %s por esse trecho: a diferença de tarifa fica %s, mais %s. Total estimado: %s." % (
            _money(p["paid"]), _money(diff), fee, _money(total)))
    else:
        fee = "sem multa (bilhete flexível)" if p["flexible"] else "mais a multa da %s de %s" % (p["airline"], _money(p["fee"]))
        lines.append("Como a reserva tem hotel, a equipe calcula a diferença de tarifa com o valor pago pelo voo, %s." % fee)
    lines.append("A equipe confirma o valor final ao remarcar.")
    return {"ok": True, "message": " ".join(lines), "total": total,
            "flight": {"airline": p["airline"], "from": p["from"], "to": p["to"], "date": p["newDate"], "departure": dep, "arrival": arr, "fare": fare}}
