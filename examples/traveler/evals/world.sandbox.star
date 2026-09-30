# A Booking.com sandbox (agency-001's world) with the cities the personas
# travel between. Flights and hotels are fixed functions of the route and
# the dates, the same for every variant. Shapes follow the real
# searchDestination, searchFlights, searchDestination (hotels) and
# searchHotels responses, as examples/traveler/evals/booking.sandbox.star.

_ACCENTS = {"ã": "a", "á": "a", "â": "a", "à": "a", "é": "e", "ê": "e", "í": "i", "ó": "o", "ô": "o", "õ": "o", "ú": "u", "ç": "c"}

def _fold(s):
    s = (s or "").lower()
    for k, v in _ACCENTS.items():
        s = s.replace(k, v)
    return s

def _matches(query, *names):
    text = " ".join([_fold(n) for n in names])
    words = _fold(query).replace(",", " ").replace("-", " ").split()
    return len(words) > 0 and all([w in text for w in words])

# city code, city name, airports (code, name), base one-way fare from São
# Paulo in BRL, flight minutes from São Paulo.
CITIES = [
    ("SAO", "São Paulo", [("GRU", "Guarulhos International Airport"), ("CGH", "Congonhas Airport")]),
    ("RIO", "Rio de Janeiro", [("GIG", "Galeão International Airport"), ("SDU", "Santos Dumont Airport")]),
    ("BHZ", "Belo Horizonte", [("CNF", "Confins International Airport")]),
    ("CWB", "Curitiba", [("CWB", "Afonso Pena International Airport")]),
    ("POA", "Porto Alegre", [("POA", "Salgado Filho International Airport")]),
    ("FLN", "Florianópolis", [("FLN", "Hercílio Luz International Airport")]),
    ("REC", "Recife", [("REC", "Recife International Airport")]),
    ("NAT", "Natal", [("NAT", "Governador Aluízio Alves International Airport")]),
    ("MCZ", "Maceió", [("MCZ", "Zumbi dos Palmares International Airport")]),
    ("SSA", "Salvador", [("SSA", "Salvador International Airport")]),
    ("FOR", "Fortaleza", [("FOR", "Pinto Martins International Airport")]),
    ("BPS", "Porto Seguro", [("BPS", "Porto Seguro Airport")]),
    ("LIS", "Lisboa", [("LIS", "Humberto Delgado Airport")]),
]

# Distance class per city: fare and flight time.
_CLASS = {"SAO": 0, "RIO": 1, "BHZ": 1, "CWB": 1, "POA": 2, "FLN": 1, "REC": 3, "NAT": 3, "MCZ": 3, "SSA": 2, "FOR": 3, "BPS": 2, "LIS": 9}

def search_flight_destination(state, query):
    out = []
    for code, name, airports in CITIES:
        city_hit = _matches(query, name)
        if city_hit and len(airports) > 1:
            out.append({"id": code + ".CITY", "name": name, "cityName": name, "type": "CITY"})
        for a, aname in airports:
            if city_hit or _matches(query, aname, name) or _fold(query) == _fold(a):
                out.append({"id": a + ".AIRPORT", "name": aname, "cityName": name, "type": "AIRPORT"})
    return out

def _city(place_id):
    code = place_id.split(".")[0]
    for c, name, airports in CITIES:
        if c == code:
            return c, airports[0][0]
        for a, _ in airports:
            if a == code:
                return c, a
    return None, None

_DAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]

def _next_day(day):
    y, m, d = [int(x) for x in day.split("-")]
    d += 1
    if d > _DAYS[m - 1]:
        d, m = 1, m + 1
        if m > 12:
            m, y = 1, y + 1
    return "%d-%s-%s" % (y, _two(m), _two(d))

def _ordinal(day):
    y, m, d = [int(x) for x in day.split("-")]
    n = y * 365 + d
    for i in range(m - 1):
        n += _DAYS[i]
    return n

def _two(n):
    return ("0" + str(n))[-2:]

def _hm(minutes):
    return _two((minutes // 60) % 24) + ":" + _two(minutes % 60)

def _leg(frm, to, dep, arr, code, name, number):
    return {"departureAirport": {"code": frm}, "arrivalAirport": {"code": to}, "departureTime": dep,
            "arrivalTime": arr, "carriersData": [{"code": code, "name": name}], "flightInfo": {"flightNumber": number}}

def _segment(frm, to, day, start, minutes, code, name, number):
    end = start + minutes
    arr_day = day if end < 24 * 60 else _next_day(day)
    dep, arr = day + "T" + _hm(start) + ":00", arr_day + "T" + _hm(end) + ":00"
    return {"departureAirport": {"code": frm}, "arrivalAirport": {"code": to}, "departureTime": dep,
            "arrivalTime": arr, "totalTime": minutes * 60, "legs": [_leg(frm, to, dep, arr, code, name, number)]}

# The four daily departures of every domestic route: (airline code, name,
# departure minute of the day, fare difference, checked bag included).
_DOMESTIC = [
    ("LA", "LATAM", 7 * 60 + 10, 250, False),
    ("G3", "GOL", 12 * 60 + 40, 0, False),
    ("AD", "Azul", 16 * 60 + 5, 80, True),
    ("G3", "GOL", 19 * 60 + 50, -60, False),
]

def search_flights(state, fromId, toId, departDate, returnDate, adults, children, cabinClass, currency):
    state.setdefault("searches", []).append({"from": fromId, "to": toId, "date": departDate, "return": returnDate,
                                             "adults": adults, "children": children, "cabin": cabinClass})
    fc, fa = _city(fromId)
    tc, ta = _city(toId)
    if fc == None or tc == None or fc == tc:
        return []
    if fromId.endswith(".AIRPORT"):
        fa = fromId.split(".")[0]
    if toId.endswith(".AIRPORT"):
        ta = toId.split(".")[0]
    kids = [int(a) for a in (children or "").split(",") if a != ""]
    payers = int(adults or 1) + len([k for k in kids if k >= 2])
    infants = len([k for k in kids if k < 2])
    dist = abs(_CLASS[fc] - _CLASS[tc]) + 1
    international = "LIS" in (fc, tc)
    day_shift = (_ordinal(departDate) * 7) % 5 * 40
    offers = []
    if international:
        schedule = [("TP", "TAP", 21 * 60 + 55, 0, True, 8 * 60 + 35), ("LA", "LATAM", 18 * 60 + 30, -350, True, 9 * 60 + 5)]
        base = 3900
    else:
        schedule = [(c, n, dep, diff, bag, 60 + dist * 50) for c, n, dep, diff, bag in _DOMESTIC]
        base = 280 + dist * 130
    for i, (code, name, dep, diff, bag, minutes) in enumerate(schedule):
        fare = base + diff + day_shift
        segs = [_segment(fa, ta, departDate, dep, minutes, code, name, code + str(1000 + i))]
        if returnDate:
            back = (dep + 5 * 60) % (22 * 60) + 6 * 60 if not international else dep
            segs.append(_segment(ta, fa, returnDate, back, minutes, code, name, code + str(2000 + i)))
            fare = fare * 2 - 40
        total = fare * payers + (fare // 10) * infants
        o = {
            "token": "%s-%s-%s-%d" % (fa, ta, departDate, i), "tripType": "ROUNDTRIP" if returnDate else "ONEWAY",
            "priceBreakdown": {"total": {"units": total, "currencyCode": "BRL"}},
            "segments": segs,
            "brandedFareInfo": {"fareName": "Plus" if bag else "Light", "cabinBaggageExcluded": False,
                                "checkinBaggageExcluded": not bag, "features": []},
            "travellerPrices": [{"travellerType": "ADULT", "travellerPriceBreakdown": {"total": {"units": fare}}}] +
                               ([{"travellerType": "INFANT", "travellerPriceBreakdown": {"total": {"units": fare // 10}}}] if infants else []),
        }
        offers.append(o)
    return offers

def _price(units, nanos = 0):
    return {"total": {"currencyCode": "BRL", "units": units, "nanos": nanos}}

def seat_map(state, offerToken, currency):
    i = int(offerToken.split("-")[-1])
    bag = [210, 180, 150, 180][i % 4]
    return {
        "checkedInBaggage": {"options": [{
            "luggageAllowance": {"luggageType": "CHECKED_IN", "maxPiece": 1, "maxWeightPerPiece": 50.7, "massUnit": "LB"},
            "priceBreakdown": _price(bag, 900000000)}]},
        "flexibleTicket": {"priceBreakdown": _price(97, 500000000)},
        "travelInsurance": {"options": {"priceBreakdown": _price(58)},
                            "content": {"benefits": ["Cancelamento da viagem", "Extravio de bagagem"]}},
    }

# Hotels: per city, (name, kind, review, reviews, nightly price, free
# cancellation, district).
HOTELS = {
    "Natal": [("Hotel Ponta Negra Beach", "Hotel", 8.9, 2100, 420, False, "Ponta Negra"),
              ("Natal Flats Ponta Negra", "Apartment", 8.6, 900, 260, True, "Ponta Negra"),
              ("ibis Natal", "Hotel", 8.1, 5400, 170, False, "Centro"),
              ("Pousada do Morro", "Guest house", 9.2, 640, 310, True, "Ponta Negra")],
    "Maceió": [("Hotel Pajuçara Mar", "Hotel", 8.7, 1800, 390, False, "Pajuçara"),
               ("Maceió Flat Ponta Verde", "Apartment", 8.4, 700, 240, True, "Ponta Verde"),
               ("ibis Maceió", "Hotel", 8.0, 4100, 160, False, "Pajuçara")],
    "Salvador": [("Hotel Farol da Barra", "Hotel", 8.8, 2500, 380, True, "Barra"),
                 ("Pousada do Pelourinho", "Guest house", 8.9, 820, 230, False, "Pelourinho"),
                 ("ibis Salvador Rio Vermelho", "Hotel", 8.1, 3900, 175, False, "Rio Vermelho")],
    "Rio de Janeiro": [("Copacabana Palace Mar", "Hotel", 9.1, 3300, 890, True, "Copacabana"),
                       ("Hotel Atlântico Copa", "Hotel", 8.3, 2700, 360, False, "Copacabana"),
                       ("ibis Rio Centro", "Hotel", 8.0, 5100, 210, False, "Centro")],
    "Lisboa": [("Hotel Avenida Palace", "Hotel", 9.0, 2100, 1000, False, "Baixa"),
               ("Lisboa Stay Apartments", "Apartment", 8.8, 900, 600, True, "Alfama"),
               ("ibis Lisboa Centro", "Hotel", 8.2, 5400, 330, False, "Saldanha")],
    "Fortaleza": [("Hotel Iracema Praia", "Hotel", 8.5, 1900, 330, True, "Iracema"),
                  ("Fortaleza Flat Meireles", "Apartment", 8.2, 600, 210, True, "Meireles"),
                  ("Hostel Beira Mar", "Hostel", 8.0, 700, 95, True, "Meireles")],
    "Recife": [("Hotel Boa Viagem Mar", "Hotel", 8.6, 2600, 350, False, "Boa Viagem"),
               ("ibis Recife Boa Viagem", "Hotel", 8.0, 4800, 165, False, "Boa Viagem")],
    "Porto Seguro": [("Pousada Taperapuan", "Guest house", 8.7, 950, 280, True, "Taperapuã"),
                     ("Hotel Arraial Bangalô", "Hotel", 8.4, 1200, 340, False, "Centro")],
    "Porto Alegre": [("Hotel Moinhos Plaza", "Hotel", 8.6, 1600, 320, True, "Moinhos de Vento")],
    "Florianópolis": [("Hotel Jurerê Beach", "Hotel", 8.8, 1400, 450, False, "Jurerê")],
}

def search_hotel_destination(state, query):
    out = []
    for i, city in enumerate(sorted(HOTELS.keys())):
        if _matches(query, city):
            out.append({"dest_id": "-%d" % (1000 + i), "dest_type": "city", "name": city, "city_name": city})
        districts = sorted({h[6]: True for h in HOTELS[city]}.keys())
        for j, d in enumerate(districts):
            if _matches(query, d, city) and not _matches(query, city):
                out.append({"dest_id": "%d" % (9000 + i * 10 + j), "dest_type": "district", "name": d, "city_name": city})
    return out

def search_hotels(state, destId, searchType, checkIn, checkOut, adults, childrenAges, rooms, currency):
    state.setdefault("hotelSearches", []).append({"dest": destId, "type": searchType, "in": checkIn, "out": checkOut,
                                                  "adults": adults, "children": childrenAges, "rooms": rooms})
    cities = sorted(HOTELS.keys())
    city, district = None, None
    if destId.startswith("-"):
        city = cities[int(destId[1:]) - 1000]
    else:
        n = int(destId) - 9000
        city = cities[n // 10]
        district = sorted({h[6]: True for h in HOTELS[city]}.keys())[n % 10]
    nights = _ordinal(checkOut) - _ordinal(checkIn)
    people = int(adults or 1) + len([a for a in (childrenAges or "").split(",") if a != ""])
    factor = 1.0 if people <= 2 else 1.35
    out = []
    for i, (name, kind, review, count, nightly, free, d) in enumerate(HOTELS[city]):
        if district != None and d != district:
            continue
        total = int(nightly * nights * factor * int(rooms or 1))
        out.append({"hotel_id": 100 + i, "property": {
            "name": name, "propertyType": kind, "reviewScore": review, "reviewCount": count,
            "priceBreakdown": {"grossPrice": {"value": total, "currency": "BRL"}},
            "freeCancellation": free, "photoUrls": [], "district": d}})
    return out
