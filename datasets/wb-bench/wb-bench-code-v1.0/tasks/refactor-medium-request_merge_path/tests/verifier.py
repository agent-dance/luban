
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
from httpx_like.request_model import RequestBuilder
try:
    def merged_request():
        b=RequestBuilder(base_headers={"User-Agent":"base","Accept":"json"}, cookies={"sid":"1"}, params=[("base","yes")])
        return b.build("get","https://e.test",headers={"accept":"html"},cookies={"theme":"dark"},params=[("q","x")])
    safe_record("method normalized", lambda: merged_request()["method"] == "GET")
    safe_record("base headers included", lambda: merged_request()["headers"]["user-agent"] == "base")
    safe_record("headers override", lambda: merged_request()["headers"]["accept"] == "html")
    safe_record("cookies merged", lambda: merged_request()["cookies"] == {"sid":"1","theme":"dark"})
    safe_record("base params first", lambda: merged_request()["params"] == [("base","yes"),("q","x")])
    safe_record("dict params accepted", lambda: RequestBuilder(params={"a":"1"}).build("GET","/",params={"b":"2"})["params"] == [("a","1"),("b","2")])
    safe_record("list params expanded", lambda: RequestBuilder().build("GET","/",params={"tag":["a","b"]})["params"] == [("tag","a"),("tag","b")])
    safe_record("none params skipped", lambda: RequestBuilder().build("GET","/",params=[("a",None),("b","2")])["params"] == [("b","2")])
    safe_record("json content-type set", lambda: RequestBuilder().build("POST","/",json={"a":1})["headers"]["content-type"] == "application/json")
    safe_record("explicit content-type preserved", lambda: RequestBuilder().build("POST","/",headers={"Content-Type":"x/custom"},json={})["headers"]["content-type"] == "x/custom")
    safe_record("body type recorded", lambda: RequestBuilder().build("POST","/",data="abc")["body_type"] == "data")
    def excl():
        try: RequestBuilder().build("POST","/",json={},data="x")
        except ValueError: return True
        return False
    safe_record("body inputs mutually exclusive", excl)
finally: write_reward()
