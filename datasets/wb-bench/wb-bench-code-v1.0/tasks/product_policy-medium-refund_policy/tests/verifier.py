
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
by={d['request_id']:d for d in decisions_doc.get('decisions',[])}
safe_record('all five requests are present', lambda: sorted(by)==['r1','r2','r3','r4','r5'])
safe_record('low usage within window is approved', lambda: by['r1']['status']=='approved' and by['r1']['refund_cents']==1200)
safe_record('annual meaningful usage goes to manual review', lambda: by['r2']['status']=='manual_review' and by['r2']['reason']=='meaningful_usage')
safe_record('late request is denied', lambda: by['r3']['status']=='denied' and by['r3']['reason']=='outside_window')
safe_record('trial zero payment is denied', lambda: by['r4']['status']=='denied' and by['r4']['reason']=='no_paid_charge')
safe_record('feature export triggers manual review', lambda: by['r5']['status']=='manual_review')
safe_record('day age is calculated', lambda: by['r1']['days_since_payment']==4 and by['r3']['days_since_payment']==40)
safe_record('currency is preserved', lambda: all(d['currency']=='USD' for d in decisions_doc['decisions']))
safe_record('summary approved count', lambda: decisions_doc['summary']['approved']==1)
safe_record('summary denied count', lambda: decisions_doc['summary']['denied']==2)
safe_record('summary manual count', lambda: decisions_doc['summary']['manual_review']==2)
safe_record('summary refund total', lambda: decisions_doc['summary']['refund_cents']==1200)
write_reward()
