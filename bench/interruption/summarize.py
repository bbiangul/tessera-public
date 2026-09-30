import json,sys,collections
from pathlib import Path
p=Path(sys.argv[1]); rows=[json.loads(s) for s in (p/'runs.jsonl').read_text().splitlines()]
out={}
for v in sorted({r['variant'] for r in rows}):
 rr=[r for r in rows if r['variant']==v];bad=[r for r in rr if not r['correct']]
 out[v]={'n':len(rr),'correct':sum(r['correct'] for r in rr),'errors':sum(bool(r.get('error')) for r in rr),'failures':[{'id':r['id'],'repeat':r['repeat'],'want':r['want'],'got':r['got'],'probabilities':r['response']['answers']['turn']['probabilities'] if r.get('response') else {}} for r in bad]}
print(json.dumps(out,ensure_ascii=False,indent=2))
