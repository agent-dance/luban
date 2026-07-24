
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

def load_json(path):
    return json.loads((ROOT/path).read_text())

def write_reward():
    passed=sum(1 for item in RESULTS if item['passed'])
    total=len(RESULTS) or 1
    reward={'overall': passed/total, 'test_pass_rate': passed/total, 'tests_passed': passed, 'tests_total': total, 'test_status': 'pass' if passed==total else 'no_pass', 'tests': RESULTS}
    (LOG_DIR/'reward.json').write_text(json.dumps(reward, indent=2, ensure_ascii=False))
    (LOG_DIR/'reward.txt').write_text(str(reward['overall']))
    print(json.dumps(reward, indent=2, ensure_ascii=False))
proc=run_cmd(['python','app/policy.py','--data','data','--out','decisions.json'])
safe_record('command exits cleanly', lambda: proc.returncode==0)
decisions_doc=load_json(Path('decisions.json'))
by={d['invite_id']:d for d in decisions_doc.get('decisions',[])}
safe_record('five invite decisions are emitted', lambda: len(by)==5)
safe_record('free account at limit is blocked', lambda: by['i1']['status']=='blocked' and by['i1']['reason']=='seat_limit_reached')
safe_record('viewer still consumes a seat under free plan', lambda: by['i2']['status']=='blocked')
safe_record('pro account with room is allowed', lambda: by['i3']['status']=='allowed' and by['i3']['reason']=='seat_available')
safe_record('trial account counts invited user only from users table status active', lambda: by['i4']['active_seats']==1 and by['i4']['status']=='allowed')
safe_record('legacy grace allows over-limit account', lambda: by['i5']['status']=='allowed' and by['i5']['reason']=='legacy_grace')
safe_record('seat limit metadata is included', lambda: by['i1']['active_seats']==4 and by['i1']['seat_limit']==3)
safe_record('summary counts allowed and blocked', lambda: decisions_doc['summary']['allowed']==3 and decisions_doc['summary']['blocked']==2)
safe_record('legacy count is tracked separately', lambda: decisions_doc['summary']['legacy_allowed']==1)
safe_record('emails and roles are preserved', lambda: by['i3']['email']=='new3@example.com' and by['i3']['role']=='admin')
safe_record('output is stable JSON object', lambda: sorted(decisions_doc.keys())==['decisions','summary'])
safe_record('no invite id is lost', lambda: sorted(by)==['i1','i2','i3','i4','i5'])
write_reward()
