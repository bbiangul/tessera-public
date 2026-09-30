# The confirmation after a profile change: what was understood, in the
# user's words, so a misunderstanding is visible at once.

_TIME = {"price": "o menor preço", "time": "chegar mais rápido", "balanced": "equilíbrio entre preço e tempo"}

def saved_message(said):
    parts = []
    if said.get("homeCity"):
        parts.append("você costuma sair de %s" % said["homeCity"])
    if said.get("timeValue"):
        parts.append("prioridade: %s" % _TIME.get(said["timeValue"], said["timeValue"]))
    if said.get("loyaltyProgram"):
        parts.append("programa de fidelidade: %s" % said["loyaltyProgram"])
    if said.get("neverMiddleSeat") == True:
        parts.append("nunca assento do meio")
    elif said.get("neverMiddleSeat") == False:
        parts.append("assento do meio tudo bem")
    if said.get("avoidedAirlines"):
        parts.append("evitar %s" % said["avoidedAirlines"])
    if said.get("hotelDealbreaker"):
        parts.append("em hotel, não tolera: %s" % said["hotelDealbreaker"])
    return "Anotado para as próximas buscas: " + "; ".join(parts) + "."
