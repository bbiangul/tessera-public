#!/usr/bin/env python3
"""Synthetic inputs for the seat-analysis parity test: scored flights and
seat maps shaped like Booking.com's getSeatMap. No real data.
Run: python3 examples/traveler/testdata/gen_seats.py"""
import json
import os

HERE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "seats")
COLUMNS = [{"id": "A", "type": "WINDOW"}, {"id": "B", "type": "BETWEEN"}, {"id": "C", "type": "AISLE"},
           {"id": "D", "type": "AISLE"}, {"id": "E", "type": "BETWEEN"}, {"id": "F", "type": "WINDOW"}]


def seat(col, free=True, price=None, normal=True, chars=None):
    s = {"colId": col, "description": "NORMAL" if normal else "OCCUPIED"}
    s["priceBreakdown"] = None if free else {"total": {"units": price, "currencyCode": "BRL"}}
    if chars:
        s["seatCharacteristic"] = chars
    return s


def row(n, seats):
    return {"rowNumber": n, "seats": seats}


def seatmap(rows, status=True):
    return {"status": status, "data": {"seatMap": {"columns": COLUMNS, "rows": rows}}}


def flight(token, secs, depart):
    return {"token": token, "totalDuration": secs, "segments": [{"departTime": depart}]}


MAPS = {
    "free-window": seatmap([row(10, [seat("A"), seat("B"), seat("C", normal=False)])]),
    "middle-only": seatmap([row(20, [seat("A", normal=False), seat("B"), seat("E")])]),
    "paid-window": seatmap([row(15, [seat("A", free=False, price=45), seat("F", free=False, price=39.9), seat("B")])]),
    "bassinet-front": seatmap([row(1, [seat("A"), seat("C", free=False, price=80)]), row(9, [seat("D")])]),
    "bassinet-mid": seatmap([row(12, [seat("A", chars=["BASSINET"]), seat("F", chars=["Bassinet", "EXIT"])]), row(13, [seat("C")])]),
    "failed": seatmap([], status=False),
}

CASES = {
    "mixed": ([flight("t1", 11700, "2026-10-12T07:10:00"), flight("t2", 18000, "2026-10-12T22:15:00"),
               flight("t3", 16200, "2026-10-12T18:30:00"), flight(None, 3000, "2026-10-12T09:00:00"),
               flight("t5", 9000, "2026-10-12T04:50:00")],
              ["free-window", "middle-only", "paid-window", "free-window", "failed"], False, False),
    "long-middle": ([flight("t1", 20000, "2026-10-12T10:00:00")], ["middle-only"], True, False),
    "infant-front": ([flight("t1", 11700, "2026-10-12T07:10:00")], ["bassinet-front"], False, True),
    # The original counts a row once per bassinet seat (a jq generator in
    # select); the port counts each seat once.
    "infant-mid": ([flight("t1", 11700, "2026-10-12T07:10:00")], ["bassinet-mid"], False, True),
}

if __name__ == "__main__":
    os.makedirs(HERE, exist_ok=True)
    for name, (flights, maps, never_middle, infants) in CASES.items():
        data = {"scoredFlights": {"topFlights": flights, "neverMiddleSeat": never_middle, "summary": {"hasInfants": infants}},
                "seatMaps": [MAPS[m] for m in maps]}
        with open(os.path.join(HERE, name + ".input.json"), "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=1)
            f.write("\n")
    print(len(CASES), "cases")
