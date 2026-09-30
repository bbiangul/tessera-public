#!/usr/bin/env python3
"""Synthetic hotel-search inputs for the score_hotels parity test, shaped
like Booking.com's searchHotels response. No real data.
Run: python3 examples/traveler/testdata/gen_score_hotels.py"""
import json
import os

HERE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "score_hotels")


def hotel(hid, name, score, count, price, ptype, free, photo=True, center="1.2 km from centre"):
    prop = {"name": name, "reviewScore": score, "reviewCount": count,
            "priceBreakdown": {"grossPrice": {"value": price, "currency": "BRL"}},
            "propertyType": ptype, "freeCancellation": free, "distanceToCenter": {"label": center},
            "checkinDate": "2026-10-12", "checkoutDate": "2026-10-15"}
    if photo:
        prop["photoUrls"] = [f"https://example.test/{hid}.jpg"]
    if score is None:
        del prop["reviewScore"]
    if price is None:
        del prop["priceBreakdown"]
    return {"hotel_id": hid, "property": prop}


HOTELS = [
    hotel(101, "Hotel Central", 8.7, 1200, 380.0, "Hotel", True),
    hotel(102, "Pousada do Porto", 9.1, 340, 210.0, "Guesthouse", False),
    hotel(103, "Apart Mar", 8.2, 90, 180.0, "Aparthotel", True),
    hotel(104, "Studio Alfama", 7.8, 45, 150.0, "Apartment", False),
    hotel(105, "Hostel Bairro", 6.9, 2000, 60.0, "Hostel", True),
    hotel(106, "Grand Palace Hotel", 9.2, 5000, 950.0, "Hotel", True),
    hotel(107, "Hotel Econômico", 7.4, 800, 120.0, "Hotel", False),
    hotel(108, "Residence Lx", 8.0, 12, 260.0, "Serviced apartment", True, photo=False),
    hotel(109, "No Reviews Inn", None, 0, 100.0, "Hotel", True),
    hotel(110, "Mystery Price", 8.9, 60, None, "Hotel", False),
    hotel(111, "Twin Hotel A", 8.5, 100, 300.0, "Hotel", False),
    hotel(112, "Twin Hotel B", 8.5, 100, 300.0, "Hotel", False),
]

CASES = {
    "hotel": ({}, "hotel"),
    "price": ({"time_value": "price"}, "hotel"),
    "flexible": ({"time_value": "balanced"}, "flexible"),
    "aparthotel": ({}, "aparthotel"),
    "apartment": ({"time_value": "price"}, "apartment"),
    # Long stays: the original's determinePropertyType returns this type,
    # which its scoring never matched (fixed in the port).
    "serviced-apartment": ({}, "serviced_apartment"),
    "dealbreaker": ({"hotel_dealbreaker": "WiFi"}, "hotel"),
}

if __name__ == "__main__":
    os.makedirs(HERE, exist_ok=True)
    for name, (prefs, ptype) in CASES.items():
        data = {"rawHotels": {"status": True, "data": {"hotels": HOTELS}},
                "userPreferences": prefs, "propertyType": {"propertyType": ptype}}
        with open(os.path.join(HERE, name + ".input.json"), "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=1)
            f.write("\n")
    print(len(CASES), "cases")
