#!/usr/bin/env python3
"""Generates synthetic flight-search inputs for the score_flights parity
test: offers shaped like the Booking.com (RapidAPI) searchFlights response,
user preferences and time constraints. No real data.
Run: python3 examples/traveler/testdata/gen_score_flights.py"""
import json
import os

HERE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "score_flights")


def leg(frm, to, dep, arr, code, name, number):
    return {"departureAirport": {"code": frm}, "arrivalAirport": {"code": to},
            "departureTime": dep, "arrivalTime": arr,
            "carriersData": [{"code": code, "name": name}], "flightInfo": {"flightNumber": number}}


def segment(frm, to, dep, arr, seconds, legs):
    return {"departureAirport": {"code": frm}, "arrivalAirport": {"code": to},
            "departureTime": dep, "arrivalTime": arr, "totalTime": seconds, "legs": legs}


def offer(token, price, segments, bags=(True, False), infant_scope=False, travelers=None):
    carry, checked = bags
    o = {"token": token, "tripType": "ONEWAY",
         "priceBreakdown": {"total": {"units": price, "currencyCode": "BRL"}},
         "segments": segments,
         "brandedFareInfo": {"fareName": "Light" if not checked else "Plus",
                             "cabinBaggageExcluded": not carry, "checkinBaggageExcluded": not checked,
                             "features": [{"featureName": "PERSONAL_BAGGAGE", "category": "BAGGAGE", "availability": "INCLUDED", "label": "1 personal item"}]
                             + ([{"featureName": "CHECKED", "category": "BAGGAGE", "availability": "INCLUDED", "label": "1 checked bag 23kg"}] if checked else [])},
         "travellerPrices": travelers or [{"travellerType": "ADULT", "travellerPriceBreakdown": {"total": {"units": price}}}]}
    if infant_scope:
        o["unifiedPriceBreakdown"] = {"items": [{"scope": "FLIGHT_ADULT", "price": {"units": price - 150}},
                                                {"items": [{"scope": "FLIGHT_INFANT", "price": {"units": 150}}]}]}
    return o


def direct(token, price, dep, arr, secs, code, name, bags=(True, False), **kw):
    return offer(token, price, [segment("GRU", "REC", dep, arr, secs, [leg("GRU", "REC", dep, arr, code, name, code + "100")])], bags, **kw)


def one_stop(token, price, dep, mid_arr, mid_dep, arr, secs, code, name, via="BSB", bags=(True, False)):
    return offer(token, price, [segment("GRU", "REC", dep, arr, secs, [
        leg("GRU", via, dep, mid_arr, code, name, code + "200"),
        leg(via, "REC", mid_dep, arr, code, name, code + "201")])], bags)


BASE = [
    direct("t-latam-am", 890, "2026-10-12T07:10:00", "2026-10-12T10:25:00", 11700, "LA", "LATAM"),
    direct("t-gol-noon", 640, "2026-10-12T12:40:00", "2026-10-12T15:55:00", 11700, "G3", "GOL"),
    direct("t-azul-pm", 720, "2026-10-12T16:05:00", "2026-10-12T19:30:00", 12300, "AD", "Azul", bags=(True, True)),
    one_stop("t-gol-stop", 410, "2026-10-12T06:00:00", "2026-10-12T07:40:00", "2026-10-12T09:50:00", "2026-10-12T12:30:00", 23400, "G3", "GOL"),
    one_stop("t-azul-late", 505, "2026-10-12T21:30:00", "2026-10-12T23:00:00", "2026-10-13T05:40:00", "2026-10-13T08:15:00", 38700, "AD", "Azul", via="CNF"),
    direct("t-latam-night", 560, "2026-10-12T23:55:00", "2026-10-13T03:10:00", 11700, "LA", "LATAM"),
    direct("t-latam-eve", 980, "2026-10-12T19:20:00", "2026-10-12T22:35:00", 11700, "LA", "LATAM", bags=(True, True)),
]


def raw(offers, status=True):
    return {"status": status, "data": {"flightOffers": offers}}


CASES = {
    "balanced": {"prefs": {}, "time": ""},
    "price": {"prefs": {"time_value": "price"}, "time": ""},
    "time": {"prefs": {"time_value": "time"}, "time": ""},
    "loyalty-preferred": {"prefs": {"loyalty_program": "Smiles e LATAM Pass, prefiro LATAM"}, "time": ""},
    "loyalty-azul": {"prefs": {"loyalty_program": "TudoAzul", "time_value": "price"}, "time": ""},
    "morning": {"prefs": {}, "time": "prefiro voos de manhã"},
    "evening": {"prefs": {"time_value": "time"}, "time": "à noite"},
    "arrive-by": {"prefs": {}, "time": "preciso chegar até 15h"},
    "depart-after": {"prefs": {}, "time": "sair depois das 10h30"},
    "no-overnight": {"prefs": {}, "time": "sem pernoite, de manhã"},
    "avoid-code": {"prefs": {"avoided_airlines": "g3"}, "time": ""},
    # Legacy bug: names are compared with codes, so GOL is not filtered.
    "avoid-name": {"prefs": {"avoided_airlines": "Gol"}, "time": ""},
    "never-middle": {"prefs": {"never_middle_seat": True, "time_value": "balanced"}, "time": ""},
}

if __name__ == "__main__":
    os.makedirs(HERE, exist_ok=True)
    for name, c in CASES.items():
        offers = list(BASE)
        if name == "never-middle":
            offers = offers[:3]
        data = {"rawFlights": raw(offers), "userPrefs": c["prefs"], "timeConstraint": c["time"]}
        with open(os.path.join(HERE, name + ".input.json"), "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=1)
            f.write("\n")
    infant = [direct("t-infant", 1200, "2026-10-12T09:00:00", "2026-10-12T12:15:00", 11700, "LA", "LATAM", infant_scope=True)] + BASE[1:3]
    with open(os.path.join(HERE, "infant.input.json"), "w", encoding="utf-8") as f:
        json.dump({"rawFlights": raw(infant), "userPrefs": {}, "timeConstraint": ""}, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print(len(CASES) + 1, "cases")
