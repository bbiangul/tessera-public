#!/usr/bin/python3
"""Partner CLI launcher: audited commands, metered live test quota; no keys in logs."""
import json,os,subprocess,sys,time,fcntl
from pathlib import Path
root=Path(__file__).resolve().parent.parent
args=sys.argv[1:]
if args and args[0] not in ('validate','explain','test','runs','help','version','--help','-h'):
 raise SystemExit('This partner kit exposes validate, explain, test, runs, help and version.')
logs=root/'usage';logs.mkdir(exist_ok=True)
live=bool(args and args[0]=='test' and '--help' not in args and '-h' not in args)
if '--cassette' in args and ('--cassette-mode' not in args or args[args.index('--cassette-mode')+1]=='replay'):live=False
with (logs/'calls.jsonl').open('a+') as f:
 fcntl.flock(f,fcntl.LOCK_EX);f.seek(0);previous=[json.loads(l) for l in f]
 if live and sum(r['live'] for r in previous)>=20:raise SystemExit('Partner live budget exhausted (20 reservations of $0.05).')
 invocation=len(previous)+1
 f.write(json.dumps({'call':invocation,'time':time.time(),'args':args,'live':live,'reservedUSD':.05 if live else 0})+'\n');f.flush()
env=os.environ.copy()
if live:
 if '--max-cost-usd' in args:
  i=args.index('--max-cost-usd');del args[i:i+2]
 args+=['--max-cost-usd','.05']
 if '--out' not in args:args+=['--out',str(logs/f'live-{invocation}.json')]
 env['TYPESAFE_API_KEY']=os.environ['TYPESAFE_API_KEY']
 env['OPENROUTER_API_KEY']=os.environ['OPENROUTER_API_KEY']
raise SystemExit(subprocess.call([str(root/'bin/tessera-cli'),*args],env=env))
