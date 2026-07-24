
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
teams={t.get('team'):t for t in report.get('teams',[]) if isinstance(t, dict)}
safe_record('ticket total correct', lambda: report['summary']['tickets']==6)
safe_record('open ticket count correct', lambda: report['summary']['open_tickets']==2)
safe_record('priority counts stable', lambda: report['summary']['priority_counts']=={'high':3,'normal':3})
safe_record('billing counts three tickets', lambda: teams['billing']['tickets']==3)
safe_record('billing average first response rounded', lambda: is_close(teams['billing']['avg_first_response_minutes'],111.67,0.01))
safe_record('billing has one high priority miss at 45 min no miss and normal 270 miss', lambda: teams['billing']['sla_misses']==1)
safe_record('tech open no response counts as miss', lambda: teams['tech']['sla_misses']==2)
safe_record('sales fast response has no miss', lambda: teams['sales']['sla_misses']==0)
safe_record('resolution average uses closed only', lambda: is_close(teams['billing']['avg_resolution_minutes'],780.0))
safe_record('team rows sorted alphabetically', lambda: [t['team'] for t in report['teams']]==['billing','sales','tech'])
safe_record('responded count excludes blank first response', lambda: teams['tech']['responded']==1)
safe_record('summary miss total is sum of teams', lambda: report['summary']['sla_misses']==3)
write_reward()
