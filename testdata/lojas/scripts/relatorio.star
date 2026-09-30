# The weekly note to the opening team, from metrics/funnel: numbers are
# read from the report, never written by a model.

# Starlark's % has no precision: round by hand.
def _d(x):
    if x >= 10:
        return str(int(x + 0.5))
    t = int(x * 10 + 0.5)
    return "%d.%d" % (t // 10, t % 10)

def relatorio(funil):
    t = funil["totals"]
    lines = ["Abertura de lojas: %d em implantação." % t["entered"]]
    for s in t["stages"]:
        conv = ""
        if s["lost"] > 0:  # without losses (no window) a rate is always 100%: say nothing
            conv = ", %d%% dos decididos" % int(100 * s["conversion"] + 0.5)
        lines.append("- %s: %d concluíram, %d em andamento%s" % (s["name"], s["reached"], s["pending"], conv))
    tempos = []
    stuck = []
    for c in funil["cohorts"]:
        if c["total"]["reached"] > 0:
            tempos.append("coorte %s: %s dias do contrato à inauguração (mediana)" % (c["start"], _d(c["total"]["medianDays"])))
        for s in c["stages"]:
            for x in s["stuck"]:
                stuck.append("loja %s parada antes de %s há %s dias" % (x["id"], s["name"], _d(x["days"])))
    lines.extend(tempos)
    if stuck:
        lines.append("Atenção: " + "; ".join(stuck) + ".")
    return "\n".join(lines)
