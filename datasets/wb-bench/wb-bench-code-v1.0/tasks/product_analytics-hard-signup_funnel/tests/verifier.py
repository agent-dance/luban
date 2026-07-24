
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

def is_close(a, b, tol=1e-6):
    return abs(float(a)-float(b)) <= tol

def write_reward():
    passed=sum(1 for item in RESULTS if item['passed'])
    total=len(RESULTS) or 1
    reward={'overall': passed/total, 'test_pass_rate': passed/total, 'tests_passed': passed, 'tests_total': total, 'test_status': 'pass' if passed==total else 'no_pass', 'tests': RESULTS}
    (LOG_DIR/'reward.json').write_text(json.dumps(reward, indent=2, ensure_ascii=False))
    (LOG_DIR/'reward.txt').write_text(str(reward['overall']))
    print(json.dumps(reward, indent=2, ensure_ascii=False))
proc=run_cmd(['python','app/report.py','--data','data','--out','report.json'])
safe_record('command exits cleanly', lambda: proc.returncode==0)
report=load_json(Path('report.json'))
ch={c['channel']:c for c in report.get('channels',[])}
safe_record('steps are stable', lambda: report['steps']==['signup','verify_email','create_project','invite_team'])
safe_record('overall users count', lambda: report['overall']['users']==6)
safe_record('signup count all users', lambda: report['overall']['steps']['signup']==6)
safe_record('verify count respects step order', lambda: report['overall']['steps']['verify_email']==5)
safe_record('create project requires prior verify in order', lambda: report['overall']['steps']['create_project']==3)
safe_record('invite count one user', lambda: report['overall']['steps']['invite_team']==1)
safe_record('overall activation rate', lambda: is_close(report['overall']['activation_rate'],0.5))
safe_record('ads activation only u1 qualifies because u2 out of order', lambda: ch['ads']['steps']['create_project']==1 and is_close(ch['ads']['activation_rate'],1/3,1e-4))
safe_record('partner activation perfect', lambda: ch['partner']['users']==1 and ch['partner']['activation_rate']==1.0)
safe_record('organic delayed create still counts if ordered', lambda: ch['organic']['steps']['create_project']==1)
safe_record('channels sorted by name', lambda: [c['channel'] for c in report['channels']]==['ads','organic','partner'])
safe_record('invite rate correct', lambda: is_close(report['overall']['invite_rate'],1/6,1e-4))
write_reward()
