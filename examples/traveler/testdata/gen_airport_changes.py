#!/usr/bin/env python3
"""Synthetic inputs for the airport-change parity tests: flight offers
shaped like Booking.com's searchFlights (detection) and detected changes
with transfer times (risk). No real data.
Run: python3 examples/traveler/testdata/gen_airport_changes.py"""
import json
import os

HERE = os.path.dirname(os.path.abspath(__file__))


def leg(frm, to, dep, arr):
    return {"departureAirport": {"code": frm}, "arrivalAirport": {"code": to}, "departureTime": dep, "arrivalTime": arr}


def offer(token, *segments):
    return {"token": token, "segments": [{"legs": list(legs)} for legs in segments]}


def raw(*offers, status=True):
    return {"status": status, "data": {"flightOffers": list(offers)}}


DETECT = {
    "no-change": raw(
        offer("direct", [leg("GRU", "REC", "2026-10-12T07:10:00", "2026-10-12T10:25:00")]),
        offer("same-airport", [leg("POA", "GRU", "2026-10-12T06:00:00", "2026-10-12T07:40:00"),
                               leg("GRU", "REC", "2026-10-12T09:00:00", "2026-10-12T12:15:00")])),
    "gru-cgh-tight": raw(
        offer("t1", [leg("REC", "GRU", "2026-10-12T10:45:00", "2026-10-12T14:00:00"),
                     leg("CGH", "CWB", "2026-10-12T15:30:00", "2026-10-12T16:40:00")])),
    "several": raw(
        offer("a", [leg("GRU", "LHR", "2026-10-12T18:00:00.000", "2026-10-13T10:00:00.000"),
                    leg("LGW", "LIS", "2026-10-13T15:00:00.000", "2026-10-13T17:45:00.000")]),
        offer("b", [leg("GIG", "JFK", "2026-10-12T22:00:00", "2026-10-13T06:30:00"),
                    leg("LGA", "ORD", "2026-10-13T08:30:00", "2026-10-13T10:15:00")]),
        offer("c", [leg("GRU", "LHR", "2026-10-12T18:00:00", "2026-10-13T10:00:00"),
                    leg("LGW", "LIS", "2026-10-13T12:00:00", "2026-10-13T14:45:00")]),
        offer("d", [leg("AAA", "BBB", "2026-10-12T08:00:00", "2026-10-12T09:00:00"),
                    leg("CCC", "DDD", "2026-10-12T11:00:00", "2026-10-12T12:00:00")])),
    "two-in-one-trip": raw(
        offer("x", [leg("REC", "GRU", "2026-10-12T06:00:00", "2026-10-12T09:15:00"),
                    leg("CGH", "SDU", "2026-10-12T11:00:00", "2026-10-12T12:00:00"),
                    leg("GIG", "LIS", "2026-10-12T20:00:00", "2026-10-13T09:00:00")],
              [leg("LIS", "GIG", "2026-10-20T10:00:00", "2026-10-20T17:00:00"),
               leg("SDU", "CGH", "2026-10-20T19:30:00", "2026-10-20T20:30:00")])),
    "failed-search": raw(status=False),
}


def change(token, frm, to, layover):
    return {"flightToken": token, "segmentIndex": 0, "legIndex": 0, "fromAirport": frm, "toAirport": to,
            "arrivalTime": "2026-10-12T14:00:00", "departureTime": "2026-10-12T15:30:00", "layoverMinutes": layover}


def transfer(frm, to, minutes, estimated=False):
    return {"from": frm, "to": to, "transferMinutes": minutes, "estimated": estimated}


RISK = {
    "no-changes": ({"airportChanges": []}, {"transferTimes": []}),
    "levels": ({"airportChanges": [
        change("high", "GRU", "CGH", 90),      # 74 + 45 = 119 > 90
        change("medium", "GRU", "CGH", 130),   # 119 <= 130 < 149
        change("low", "GRU", "CGH", 149),      # exactly the standard buffer
        change("unknown", "JFK", "LGA", 150),  # no transfer: 90 + 45 = 135 <= 150 < 165
    ]}, {"transferTimes": [transfer("GRU", "CGH", 74), transfer("LHR", "LGW", 95)]}),
    "worst-per-flight": ({"airportChanges": [
        change("x", "GRU", "CGH", 300), change("x", "SDU", "GIG", 60), change("y", "LHR", "LGW", 200.5),
    ]}, {"transferTimes": [transfer("GRU", "CGH", 74), transfer("SDU", "GIG", 40), transfer("LHR", "LGW", 95)]}),
    "missing-data": ({"nothing": True}, {"transferTimes": []}),
}


def write(sub, name, value):
    with open(os.path.join(HERE, sub, name + ".input.json"), "w", encoding="utf-8") as f:
        json.dump(value, f, ensure_ascii=False, indent=1, sort_keys=True)
        f.write("\n")


for name, flights in DETECT.items():
    write("airport_changes", name, {"rawFlights": flights})
for name, (changes, transfers) in RISK.items():
    write("airport_risk", name, {"rawFlights": raw(), "airportChangeData": changes, "transferTimeData": transfers})
