
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
cohorts={c['cohort_date']:c for c in report.get('cohorts',[])}
safe_record('summary users count', lambda: report['summary']['users']==5)
safe_record('summary retained count', lambda: report['summary']['retained_day7']==2)
safe_record('summary retention rate', lambda: is_close(report['summary']['retention_rate'],0.4))
safe_record('three cohort rows', lambda: [c['cohort_date'] for c in report['cohorts']]==['2026-06-01','2026-06-02','2026-06-03'])
safe_record('june1 has u1 retained exactly day7 not u2 day8', lambda: cohorts['2026-06-01']['retained_day7']==1)
safe_record('june1 retention rate half', lambda: is_close(cohorts['2026-06-01']['retention_rate'],0.5))
safe_record('june2 has u3 retained', lambda: cohorts['2026-06-02']['retained_day7']==1)
safe_record('june3 has no retained users', lambda: cohorts['2026-06-03']['retained_day7']==0)
safe_record('plan split exists for free and pro', lambda: set(cohorts['2026-06-01']['plans'])=={'free','pro'})
safe_record('plan retained counts correct', lambda: cohorts['2026-06-02']['plans']['free']['retained_day7']==1 and cohorts['2026-06-02']['plans']['pro']['retained_day7']==0)
safe_record('same day activity alone not retained', lambda: cohorts['2026-06-03']['plans']['free']['retention_rate']==0.0)
safe_record('cohort object fields stable', lambda: sorted(report['cohorts'][0].keys())==['cohort_date','plans','retained_day7','retention_rate','users'])
write_reward()
