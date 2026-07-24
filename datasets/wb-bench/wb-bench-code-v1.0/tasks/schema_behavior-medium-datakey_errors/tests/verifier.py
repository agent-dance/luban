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

from marshmallow_like.datakey import Field, Nested, Schema

profile_schema = Schema({"email": Field(str, required=True, data_key="emailAddress"), "age": Field(int, required=True, data_key="ageYears")})
item_schema = Schema({"sku": Field(str, required=True, data_key="skuId"), "qty": Field(int, required=True, data_key="quantity")})
user_schema = Schema({
    "user_id": Field(int, required=True, data_key="userId"),
    "profile": Nested(profile_schema, required=True, data_key="profileData"),
    "items": Nested(item_schema, required=True, data_key="lineItems", many=True),
})


def errors(data):
    return user_schema.load(data)[1]


def output(data):
    return user_schema.load(data)[0]


def locs(data):
    return [error["loc"] for error in errors(data)]

try:
    safe_record("missing top uses data key", lambda: locs({"profileData": {"emailAddress": "a", "ageYears": 1}, "lineItems": []})[0] == ["userId"])
    safe_record("missing nested parent uses data key", lambda: locs({"userId": 1, "lineItems": []})[0] == ["profileData"])
    safe_record("missing many parent uses data key", lambda: locs({"userId": 1, "profileData": {"emailAddress": "a", "ageYears": 1}})[0] == ["lineItems"])
    safe_record("invalid top uses data key", lambda: locs({"userId": "1", "profileData": {"emailAddress": "a", "ageYears": 1}, "lineItems": []})[0] == ["userId"])
    safe_record("missing nested uses data key", lambda: locs({"userId": 1, "profileData": {"ageYears": 1}, "lineItems": []})[0] == ["profileData", "emailAddress"])
    safe_record("invalid nested uses data key", lambda: locs({"userId": 1, "profileData": {"emailAddress": "a", "ageYears": "x"}, "lineItems": []})[0] == ["profileData", "ageYears"])
    safe_record("missing many item uses data key", lambda: locs({"userId": 1, "profileData": {"emailAddress": "a", "ageYears": 1}, "lineItems": [{"quantity": 1}]})[0] == ["lineItems", 0, "skuId"])
    safe_record("invalid many item uses data key", lambda: locs({"userId": 1, "profileData": {"emailAddress": "a", "ageYears": 1}, "lineItems": [{"skuId": "a", "quantity": 1}, {"skuId": "b", "quantity": "x"}]})[0] == ["lineItems", 1, "quantity"])
    safe_record("unknown nested uses data key parent", lambda: locs({"userId": 1, "profileData": {"emailAddress": "a", "ageYears": 1, "debug": True}, "lineItems": []})[0] == ["profileData", "debug"])
    safe_record("multiple top and nested errors use external names", lambda: locs({"profileData": {"ageYears": "x"}, "lineItems": [{"quantity": "bad"}]}) == [["userId"], ["profileData", "emailAddress"], ["profileData", "ageYears"], ["lineItems", 0, "skuId"], ["lineItems", 0, "quantity"]])
    safe_record("second many item missing keeps index", lambda: locs({"userId": 1, "profileData": {"emailAddress": "a", "ageYears": 1}, "lineItems": [{"skuId": "a", "quantity": 1}, {}]}) == [["lineItems", 1, "skuId"], ["lineItems", 1, "quantity"]])
    safe_record("external names do not leak internal many name", lambda: all(loc[0] != "items" for loc in locs({"userId": 1, "profileData": {"emailAddress": "a", "ageYears": 1}, "lineItems": [{}]})))
    safe_record("many output keeps existing structure", lambda: "items" not in output({"userId": 2, "profileData": {"emailAddress": "b", "ageYears": 3}, "lineItems": [{"skuId": "s", "quantity": 1}]}))
finally:
    write_reward()
