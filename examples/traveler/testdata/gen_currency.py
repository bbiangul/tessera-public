#!/usr/bin/env python3
"""Synthetic inputs for the currency parity test (determineCurrency).
Cases marked * differ on purpose (scripts/currency.star, differences 1-2).
Run: python3 examples/traveler/testdata/gen_currency.py"""
import json
import os

HERE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "currency")
CASES = {
    "home-sao-paulo": ("São Paulo", "", ""),
    "home-london": ("London", "", ""),
    "home-paris-oneway": ("Paris", "Rio de Janeiro", ""),
    "no-home-roundtrip-origin-lisbon": ("", "Lisbon", "2026-10-20"),
    "no-home-oneway-origin-lisbon": ("", "Lisbon", ""),
    "no-home-roundtrip-true": ("", "New York", "true"),
    "star-atlantis": ("Atlantis", "Nowhere", "2026-10-20"),  # * original: USD (atl)
    "nothing": ("", "", ""),
    "home-airport-code": ("GRU", "", ""),
    "home-tokyo": ("Tokyo", "", ""),
    "home-accented-bogota": ("Bogotá", "", ""),
    "home-cancun": ("Cancún", "", ""),
    "star-pernambuco": ("Pernambuco", "", ""),         # * original: AUD (per)
    "home-santa-maria": ("Santa Maria", "", ""),
    "home-lisboa": ("Lisboa", "", ""),                 # the original gets it by accident: "lis"
    "star-londres-roundtrip": ("", "Londres", "2026-10-20"),  # * original: BRL
}

os.makedirs(HERE, exist_ok=True)
for name, (home, origin, rt) in CASES.items():
    with open(os.path.join(HERE, name + ".input.json"), "w", encoding="utf-8") as f:
        json.dump({"homeCity": home, "originCity": origin, "isRoundTrip": rt}, f, ensure_ascii=False, indent=1, sort_keys=True)
        f.write("\n")
