# Sandbox of the Booking.com plugin for the traveler EvalSuite: a few
# places and a fixed set of flights on any date. Shapes follow the real
# searchDestination and searchFlights responses.

_ACCENTS = {"ã": "a", "á": "a", "â": "a", "à": "a", "é": "e", "ê": "e", "í": "i", "ó": "o", "ô": "o", "õ": "o", "ú": "u", "ç": "c"}

def _fold(s):
    s = (s or "").lower()
    for k, v in _ACCENTS.items():
        s = s.replace(k, v)
    return s

def _matches(query, *names):
    # Every word of the query appears in the place's names, as a search
    # engine would match "Londres Heathrow".
    text = " ".join([_fold(n) for n in names])
    return all([w in text for w in _fold(query).replace(",", " ").split()])

def search_flight_destination(state, query):
    return [p for p in state["places"] if _matches(query, p["name"], p["cityName"])]

def _leg(frm, to, dep, arr, code, name):
    return {"departureAirport": {"code": frm}, "arrivalAirport": {"code": to}, "departureTime": dep,
            "arrivalTime": arr, "carriersData": [{"code": code, "name": name}], "flightInfo": {"flightNumber": code + "100"}}

def _offer(token, price, day, dep, arr, secs, code, name, frm, to, checked = False):
    d, a = day + "T" + dep + ":00", day + "T" + arr + ":00"
    return {
        "token": token, "tripType": "ONEWAY",
        "priceBreakdown": {"total": {"units": price, "currencyCode": "BRL"}},
        "segments": [{"departureAirport": {"code": frm}, "arrivalAirport": {"code": to}, "departureTime": d,
                      "arrivalTime": a, "totalTime": secs, "legs": [_leg(frm, to, d, a, code, name)]}],
        "brandedFareInfo": {"fareName": "Plus" if checked else "Light", "cabinBaggageExcluded": False,
                            "checkinBaggageExcluded": not checked, "features": []},
        "travellerPrices": [],
    }

def _connecting(token, price, day, first, second, code, name, checked = False):
    # first and second: (from, to, departs, arrives), times on day.
    legs = [_leg(l[0], l[1], day + "T" + l[2] + ":00", day + "T" + l[3] + ":00", code, name) for l in (first, second)]
    o = _offer(token, price, day, first[2], second[3], 0, code, name, first[0], second[1], checked)
    o["segments"][0]["legs"] = legs
    o["segments"][0]["totalTime"] = 6 * 3600
    return o

def search_flights(state, fromId, toId, departDate, returnDate, adults, children, cabinClass, currency):
    state["searches"].append({"from": fromId, "to": toId, "date": departDate, "adults": adults, "children": children, "cabin": cabinClass})
    if toId not in state["routes"]:
        return []
    if toId == "CWB.AIRPORT":
        # The cheapest lands at GRU and flies out of CGH 90 minutes later;
        # the other changes planes at CGH.
        return [
            _connecting("c1", 420, departDate, ("REC", "GRU", "10:45", "14:00"), ("CGH", "CWB", "15:30", "16:40"), "G3", "GOL"),
            _connecting("c2", 610, departDate, ("REC", "CGH", "09:00", "12:15"), ("CGH", "CWB", "14:00", "15:10"), "LA", "LATAM"),
            _connecting("c3", 680, departDate, ("REC", "GRU", "06:00", "09:15"), ("CGH", "CWB", "13:00", "14:10"), "AD", "Azul"),
        ]
    frm, to = "GRU", toId.split(".")[0]
    return [
        _offer("t1", 890, departDate, "07:10", "10:25", 11700, "LA", "LATAM", frm, to),
        _offer("t2", 640, departDate, "12:40", "15:55", 11700, "G3", "GOL", frm, to),
        _offer("t3", 720, departDate, "16:05", "19:30", 12300, "AD", "Azul", frm, to, checked = True),
    ]

def search_hotel_destination(state, query):
    return [p for p in state["hotelPlaces"] if _matches(query, p["name"], p["city_name"])]

def _hotel(id, name, kind, review, count, total, free):
    return {"hotel_id": id, "property": {
        "name": name, "propertyType": kind, "reviewScore": review, "reviewCount": count,
        "priceBreakdown": {"grossPrice": {"value": total, "currency": "BRL"}},
        "freeCancellation": free, "photoUrls": []}}

def search_hotels(state, destId, searchType, checkIn, checkOut, adults, childrenAges, rooms, currency):
    state["hotelSearches"].append({"dest": destId, "type": searchType, "in": checkIn, "out": checkOut,
                                   "adults": adults, "children": childrenAges, "rooms": rooms})
    if destId not in state["hotelCities"]:
        return []
    # Totals for the stay: the ibis is the cheapest per night, the Avenida
    # the best reviewed, the apartments win long stays; the hostel is
    # reviewed below 7 and never shown.
    return [
        _hotel(101, "Hotel Avenida Palace", "Hotel", 9.0, 2100, 3000, False),
        _hotel(102, "ibis Lisboa Centro", "Hotel", 8.2, 5400, 510, False),
        _hotel(103, "Lisboa Stay Apartments", "Apartment", 8.8, 900, 1800, True),
        _hotel(104, "Hostel da Esquina", "Hostel", 6.9, 300, 240, True),
    ]

def _price(units, nanos = 0):
    return {"total": {"currencyCode": "BRL", "units": units, "nanos": nanos}}

def seat_map(state, offerToken, currency):
    # Extras on sale for an offer, in getSeatMap's shape: a checked bag (in
    # pounds, as the API sends), the flexible ticket and insurance.
    if state.get("seatMapDown"):
        definite("UNAVAILABLE", "seat map is down")
    bag = {"t1": 210, "t2": 180, "t3": 150}.get(offerToken, 199)
    return {
        "checkedInBaggage": {"options": [{
            "luggageAllowance": {"luggageType": "CHECKED_IN", "maxPiece": 1, "maxWeightPerPiece": 50.7, "massUnit": "LB"},
            "priceBreakdown": _price(bag, 900000000)}]},
        "flexibleTicket": {"priceBreakdown": _price(97, 500000000)},
        "travelInsurance": {"options": {"priceBreakdown": _price(58)},
                            "content": {"benefits": ["Cancelamento da viagem", "Extravio de bagagem"]}},
    }
