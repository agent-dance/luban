
import csv
import json
import os
import subprocess
import sys
from pathlib import Path

ROOT=Path(os.environ.get('WORKSPACE','/workspace'))
LOG_DIR=Path(os.environ.get('LOG_DIR','/logs/verifier'))
LOG_DIR.mkdir(parents=True, exist_ok=True)
sys.path.insert(0, str(ROOT))
RESULTS=[]

def record(name, passed, detail=''):
    RESULTS.append({'name': name, 'passed': bool(passed), 'detail': str(detail)})

def safe_record(name, check):
    try:
        record(name, check())
    except Exception as exc:
        record(name, False, f'{type(exc).__name__}: {exc}')

def run_cmd(args):
    return subprocess.run(args, cwd=ROOT, text=True, capture_output=True, timeout=20)

def write_reward():
    passed=sum(1 for item in RESULTS if item['passed'])
    total=len(RESULTS) or 1
    reward={'overall': passed/total, 'test_pass_rate': passed/total, 'tests_passed': passed, 'tests_total': total, 'test_status': 'pass' if passed==total else 'no_pass', 'tests': RESULTS}
    (LOG_DIR/'reward.json').write_text(json.dumps(reward, indent=2, ensure_ascii=False))
    (LOG_DIR/'reward.txt').write_text(str(reward['overall']))
    print(json.dumps(reward, indent=2, ensure_ascii=False))
proc=run_cmd(['python','app/export.py','--data','data','--out','export.csv'])
safe_record('command exits cleanly', lambda: proc.returncode==0)
rows=list(csv.DictReader((ROOT/'export.csv').open(newline='',encoding='utf-8')))
safe_record('duplicate event removed', lambda: len(rows)==4)
safe_record('header stable', lambda: list(rows[0].keys())==['ts','event_id','event','risk','user_id','email','role','team','ip'])
by={r.get('event_id'):r for r in rows if r.get('event_id')}
safe_record('user email joined', lambda: by['e1']['email']=='a@example.com')
safe_record('delete risk high', lambda: by['e3']['risk']=='high')
safe_record('download risk medium', lambda: by['e2']['risk']=='medium')
safe_record('login default risk low', lambda: by['e4']['risk']=='low')
safe_record('role and team joined', lambda: by['e3']['role']=='admin' and by['e3']['team']=='sales')
safe_record('rows sorted by timestamp', lambda: [r['event_id'] for r in rows]==['e1','e2','e3','e4'])
safe_record('ip preserved', lambda: by['e2']['ip']=='10.0.0.1')
safe_record('event_id duplicate later row skipped', lambda: 'e5' not in by)
safe_record('csv has header plus four rows', lambda: len((ROOT/'export.csv').read_text().splitlines())==5)
safe_record('no blank risk values', lambda: all(r['risk'] for r in rows))
write_reward()
