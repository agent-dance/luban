
import json
import os
import sys
from pathlib import Path
sys.dont_write_bytecode = True
ROOT = Path(os.environ.get("WORKSPACE", "/workspace"))
LOG_DIR = Path(os.environ.get("LOG_DIR", "/logs/verifier"))
LOG_DIR.mkdir(parents=True, exist_ok=True)
sys.path.insert(0, str(ROOT))
RESULTS = []
def record(name, passed, detail=""):
    RESULTS.append({"name": name, "passed": bool(passed), "detail": str(detail)})
def safe_record(name, check):
    try:
        record(name, check())
    except Exception as exc:
        record(name, False, f"{type(exc).__name__}: {exc}")
def write_reward():
    passed = sum(1 for item in RESULTS if item["passed"])
    total = len(RESULTS) or 1
    reward = {"overall": passed / total, "test_pass_rate": passed / total, "tests_passed": passed, "tests_total": total, "test_status": "pass" if passed == total else "no_pass", "tests": RESULTS}
    (LOG_DIR / "reward.json").write_text(json.dumps(reward, indent=2, ensure_ascii=False))
    (LOG_DIR / "reward.txt").write_text(str(reward["overall"]))
    print(json.dumps(reward, indent=2, ensure_ascii=False))
from mkdocs_like.config import validate_config
try:
    cfg, errors=validate_config({'site_name':'Docs','theme':'readthedocs','plugins':['search'], 'nav':['index.md']})
    safe_record("valid config no errors", lambda: errors == [])
    safe_record("site name kept", lambda: cfg['site_name']=='Docs')
    safe_record("theme string normalized", lambda: cfg['theme']=={'name':'readthedocs'})
    safe_record("plugins kept", lambda: cfg['plugins']==['search'])
    safe_record("nav kept", lambda: cfg['nav']==['index.md'])
    cfg2, errors2=validate_config({'site_name':'Docs','theme':{'name':'material','palette':'dark'}, 'extra':{'x':1}})
    safe_record("theme mapping kept", lambda: cfg2['theme']['name']=='material' and cfg2['theme']['palette']=='dark')
    safe_record("extra mapping kept", lambda: cfg2['extra']=={'x':1})
    safe_record("defaults applied", lambda: validate_config({})[0]['theme']=={'name':'mkdocs'})
    bad_cfg={'site_name':123,'theme':[], 'plugins':'search','nav':[1], 'extra':[], 'bad': True}
    _, bad=validate_config(bad_cfg)
    paths=[e['path'] for e in bad]
    safe_record("unknown path reported", lambda: ['bad'] in paths)
    safe_record("site name type path", lambda: ['site_name'] in paths)
    safe_record("theme type path", lambda: ['theme'] in paths)
    safe_record("plugins type path", lambda: ['plugins'] in paths)
    safe_record("nav item path", lambda: ['nav',0] in paths)
    safe_record("extra type path", lambda: ['extra'] in paths)
    safe_record("messages are present", lambda: all(e['message'] for e in bad))
finally: write_reward()
