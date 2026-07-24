
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
by={d['user_id']:d for d in decisions_doc.get('decisions',[])}
safe_record('all users evaluated', lambda: sorted(by)==['u1','u2','u3','u4','u5','u6'])
safe_record('churned US old enough gets coupon', lambda: by['u1']['status']=='eligible' and by['u1']['coupon']=='WINBACK20')
safe_record('active user blocked', lambda: by['u2']['reason']=='not_churned')
safe_record('unsupported country blocked', lambda: by['u3']['reason']=='unsupported_country')
safe_record('suppressed user blocked even if high value', lambda: by['u4']['reason']=='suppressed')
safe_record('recent coupon blocks resend', lambda: by['u5']['reason']=='recent_coupon')
safe_record('recently active churned user blocked', lambda: by['u6']['reason']=='too_recently_active')
safe_record('days since seen calculated from fixed date', lambda: by['u1']['days_since_seen']==106)
safe_record('summary eligible count', lambda: decisions_doc['summary']['eligible']==1)
safe_record('summary blocked count', lambda: decisions_doc['summary']['blocked']==5)
safe_record('high value eligible count', lambda: decisions_doc['summary']['high_value_eligible']==1)
safe_record('blocked users have null coupon', lambda: all(v['coupon'] is None for k,v in by.items() if k!='u1'))
write_reward()
