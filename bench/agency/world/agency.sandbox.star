# agency-001's world: the agency's back office. Booking requests, what the
# team records, and each customer's bookings.

def _public(b):
    return {k: v for k, v in b.items() if k not in ("customer", "passengers", "email", "flight", "hotel", "conversation")}

def my_bookings(state, customer):
    return [_public(b) for b in state["bookings"] if b["customer"] == customer]

def request_booking(state, key, customer, conversation, summary, flight, hotel, passengers, email, total, departDate, extras = None):
    for b in state["bookings"]:
        if b["key"] == key:
            return _public(b)
    b = {"id": "P%d" % state["next"], "key": key, "customer": customer, "conversation": conversation, "status": "requested",
         "summary": summary, "total": total, "departDate": departDate, "passengers": passengers, "email": email,
         "flight": flight, "hotel": hotel}
    state["next"] += 1
    state["bookings"].append(b)
    return _public(b)

def request_change(state, key, customer, locator, leg, newDate, flight, estimate):
    changes = state.setdefault("changes", [])
    for c in changes:
        if c["key"] == key:
            return c
    c = {"key": key, "customer": customer, "locator": locator, "leg": leg, "newDate": newDate,
         "flight": flight, "estimate": estimate, "status": "requested"}
    changes.append(c)
    return c

def update_booking(state, id, status, locator, payment, eticket, note):
    for b in state["bookings"]:
        if b["id"] == id:
            b["status"] = status
            for k, v in (("locator", locator), ("payment", payment), ("eticket", eticket), ("note", note)):
                if v != None:
                    b[k] = v
            return _public(b)
    definite("NOT_FOUND", "no booking " + id)
