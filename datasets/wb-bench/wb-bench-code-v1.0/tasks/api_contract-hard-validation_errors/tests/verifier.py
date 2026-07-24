
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


def raises(exc_type, fn, contains=None):
    try:
        fn()
    except exc_type as exc:
        return contains is None or contains in str(exc)
    except Exception:
        return False
    return False


def write_reward():
    passed = sum(1 for item in RESULTS if item["passed"])
    total = len(RESULTS) or 1
    reward = {
        "overall": passed / total,
        "test_pass_rate": passed / total,
        "tests_passed": passed,
        "tests_total": total,
        "test_status": "pass" if passed == total else "no_pass",
        "tests": RESULTS,
    }
    (LOG_DIR / "reward.json").write_text(json.dumps(reward, indent=2, ensure_ascii=False))
    (LOG_DIR / "reward.txt").write_text(str(reward["overall"]))
    print(json.dumps(reward, indent=2, ensure_ascii=False))


from pydantic_like.errors import ValidationError
from pydantic_like.fields import Field
from pydantic_like.model import Model

model=Model({"name":Field(str, alias="fullName"),"age":Field(int),"active":Field(bool, required=False, default=True)})
try:
    safe_record("valid data returns internal field names", lambda: model.validate({"fullName":"Ada","age":3}) == {"name":"Ada","age":3,"active":True})
    def missing():
        try: model.validate({"age":3})
        except ValidationError as exc: return exc.errors()[0]["loc"] == ("fullName",) and exc.errors()[0]["type"] == "missing"
        return False
    safe_record("missing alias has stable error", missing)
    def wrong_type():
        try: model.validate({"fullName":"Ada","age":"3"})
        except ValidationError as exc: return exc.errors()[0]["loc"] == ("age",) and exc.errors()[0]["type"] == "int_type" and exc.errors()[0]["input"] == "3"
        return False
    safe_record("wrong type has stable input", wrong_type)
    def multiple_errors():
        try: model.validate({"fullName":7,"extra":1})
        except ValidationError as exc: return [e["type"] for e in exc.errors()] == ["int_type","missing","extra_forbidden"] or len(exc.errors()) == 3
        return False
    safe_record("multiple errors are accumulated", multiple_errors)
    def extra_error():
        try: model.validate({"fullName":"Ada","age":3,"extra":1})
        except ValidationError as exc: return exc.errors()[0]["loc"] == ("extra",) and exc.errors()[0]["type"] == "extra_forbidden"
        return False
    safe_record("extra fields are reported", extra_error)
    safe_record("explicit optional value is preserved", lambda: model.validate({"fullName":"Ada","age":3,"active":False})["active"] is False)
    def errors_are_copied():
        try: model.validate({"age":"bad"})
        except ValidationError as exc:
            first=exc.errors(); first.append({"loc":("x",)})
            return len(exc.errors()) == 2
        return False
    safe_record("errors returns copy", errors_are_copied)
    safe_record("bool is not accepted as int field", lambda: raises(ValidationError, lambda: model.validate({"fullName":"Ada","age":True})))
    safe_record("alias is used in output only for errors", lambda: "fullName" not in model.validate({"fullName":"Ada","age":1}))
    simple=Model({"x":Field(int, required=False, default=5)})
    safe_record("missing optional default works", lambda: simple.validate({}) == {"x":5})
    safe_record("empty model rejects extra", lambda: raises(ValidationError, lambda: Model({}).validate({"x":1})))
    safe_record("error object string is stable", lambda: "validation failed" in str(ValidationError([])))
    def alias_field_name_rejected():
        try: model.validate({"name":"Ada","age":3})
        except ValidationError as exc:
            types = [e["type"] for e in exc.errors()]
            locs = [e["loc"] for e in exc.errors()]
            return "missing" in types and "extra_forbidden" in types and ("fullName",) in locs and ("name",) in locs
        return False
    safe_record("alias does not also accept field name", alias_field_name_rejected)
    def many_errors_keep_order_and_inputs():
        try: model.validate({"fullName":5,"age":True,"z":9})
        except ValidationError as exc:
            errors = exc.errors()
            return [e["loc"] for e in errors] == [("fullName",), ("age",), ("z",)] and [e["input"] for e in errors] == [5, True, 9]
        return False
    safe_record("many errors keep stable order and inputs", many_errors_keep_order_and_inputs)
finally:
    write_reward()
