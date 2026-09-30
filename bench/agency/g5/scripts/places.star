# Choosing the airport or city a user means, with the original traveler's
# rules for the origin: a single result wins; otherwise one that names the
# user's home city; otherwise a whole city (all its airports); otherwise
# the first. No result is a failure the Function's onError explains.

def pick_place(places, home_city = ""):
    if not places:
        fail("NO_PLACE")
    if len(places) == 1:
        return places[0]
    home = (home_city or "").lower()
    if home:
        for p in places:
            if home in (p.get("name") or "").lower() or home in (p.get("cityName") or "").lower():
                return p
    for p in places:
        if p.get("type") == "CITY":
            return p
    return places[0]

def route(from_places, to_places, home_city = ""):
    """Picks both ends. Fails with NO_ORIGIN or NO_DESTINATION."""
    if not from_places:
        fail("NO_ORIGIN")
    if not to_places:
        fail("NO_DESTINATION")
    a = pick_place(from_places, home_city)
    b = pick_place(to_places, "")
    return {"fromId": a["id"], "fromName": a.get("name"), "toId": b["id"], "toName": b.get("name")}
