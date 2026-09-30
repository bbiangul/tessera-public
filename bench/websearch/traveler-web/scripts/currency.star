# The currency of a search: a port of the legacy platform's determineCurrency (bash),
# compared with the original in scoring_test.go. The user's home city
# decides; for a round trip without one, the origin; otherwise BRL.
# Intended differences (bugs fixed):
#
#   1. Airport codes and other short words match whole words only: the
#      original matched them anywhere, so "Pernambuco" was Perth (AUD, from
#      "per") and "Atlantis" Atlanta (USD, from "atl").
#   2. Portuguese names are known (Londres, Nova York...): the original
#      expected homeCity translated to English, and the profile keeps what
#      the user wrote ("Lisboa" only worked by accident, through "lis").

_FOLD = {"á": "a", "à": "a", "â": "a", "ã": "a", "é": "e", "è": "e", "ê": "e", "í": "i", "ì": "i", "î": "i",
         "ó": "o", "ò": "o", "ô": "o", "õ": "o", "ú": "u", "ù": "u", "û": "u", "ç": "c"}

# In the original's order; the first match wins. Words of 3 letters or
# fewer are whole words (difference 1); the Portuguese names are
# difference 2.
_TABLE = [
    ("BRL", ["sao paulo", "rio", "brasilia", "belo horizonte", "salvador", "fortaleza", "curitiba", "recife",
             "porto alegre", "manaus", "campinas", "goiania", "florianopolis", "gru", "gig", "bsb", "cgh", "sdu",
             "vcp", "brazil", "brasil"]),
    ("USD", ["new york", "los angeles", "miami", "chicago", "houston", "phoenix", "san francisco", "seattle", "boston",
             "atlanta", "dallas", "denver", "orlando", "las vegas", "jfk", "lax", "mia", "ord", "iah", "sfo", "sea",
             "bos", "atl", "dfw", "den", "mco", "ewr", "lga", "usa", "united states",
             "nova york", "nova iorque", "estados unidos", "eua"]),
    ("GBP", ["london", "manchester", "birmingham", "edinburgh", "glasgow", "lhr", "lgw", "man", "edi",
             "united kingdom", "england", "scotland", "londres", "edimburgo", "reino unido", "inglaterra", "escocia"]),
    ("EUR", ["paris", "berlin", "madrid", "rome", "amsterdam", "barcelona", "munich", "milan", "frankfurt", "vienna",
             "brussels", "lisbon", "dublin", "athens", "cdg", "ory", "ber", "mad", "fco", "ams", "bcn", "muc", "mxp",
             "fra", "vie", "bru", "lis", "dub", "ath", "france", "germany", "spain", "italy", "netherlands",
             "portugal", "ireland", "austria", "belgium", "greece",
             "berlim", "madri", "roma", "amsterda", "munique", "milao", "viena", "bruxelas", "lisboa",
             "atenas", "franca", "alemanha", "espanha", "italia", "holanda", "irlanda", "belgica", "grecia"]),
    ("ARS", ["buenos aires", "cordoba", "mendoza", "eze", "aep", "argentina"]),
    ("MXN", ["mexico", "cancun", "guadalajara", "monterrey", "mex", "cun", "gdl", "mty"]),
    ("JPY", ["tokyo", "osaka", "kyoto", "nagoya", "nrt", "hnd", "kix", "japan", "toquio", "japao"]),
    ("CNY", ["beijing", "shanghai", "guangzhou", "shenzhen", "pek", "pvg", "can", "szx", "china", "pequim", "xangai"]),
    ("AUD", ["sydney", "melbourne", "brisbane", "perth", "syd", "mel", "bne", "per", "australia"]),
    ("CAD", ["toronto", "vancouver", "montreal", "calgary", "yyz", "yvr", "yul", "yyc", "canada"]),
    ("CLP", ["santiago", "scl", "chile"]),
    ("COP", ["bogota", "medellin", "bog", "mde", "colombia"]),
    ("PEN", ["lima", "lim", "peru"]),
    ("CHF", ["zurich", "geneva", "zrh", "gva", "switzerland", "swiss", "genebra", "suica"]),
]

def _fold(s):
    s = (s or "").lower()
    for k, v in _FOLD.items():
        s = s.replace(k, v)
    return s

def _has(text, word):
    if len(word) > 3:
        return word in text
    padded = " " + "".join([c if c.isalnum() else " " for c in text.elems()]) + " "
    return (" " + word + " ") in padded

def currency_of(city):
    """The currency of a city or airport, or "" when unknown."""
    text = _fold(city)
    if not text:
        return ""
    for code, words in _TABLE:
        for w in words:
            if _has(text, w):
                return code
    return ""

def currency(home_city = "", origin = "", round_trip = False):
    c = currency_of(home_city)
    if c == "" and round_trip:
        c = currency_of(origin)
    return c or "BRL"
