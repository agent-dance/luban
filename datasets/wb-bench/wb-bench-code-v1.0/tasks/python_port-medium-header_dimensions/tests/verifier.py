import json
import os
import struct
import subprocess
import sys
import tempfile
from pathlib import Path

sys.dont_write_bytecode = True

ROOT = Path(os.environ.get("WORKSPACE", "/workspace"))
TARGET = ROOT / "target_python"
LOG_DIR = Path(os.environ.get("LOG_DIR", "/logs/verifier"))
LOG_DIR.mkdir(parents=True, exist_ok=True)
RESULTS = []


def record(name, passed, detail=""):
    RESULTS.append({"name": name, "passed": bool(passed), "detail": str(detail)})


def safe_record(name, check):
    try:
        record(name, check())
    except Exception as exc:
        record(name, False, f"{type(exc).__name__}: {exc}")


def run_cmd(args, cwd=TARGET, timeout=30):
    env = os.environ.copy()
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    return subprocess.run(args, cwd=str(cwd), text=True, capture_output=True, timeout=timeout, env=env)


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


def png(width, height):
    return (
        b"\x89PNG\r\n\x1a\n"
        + (13).to_bytes(4, "big")
        + b"IHDR"
        + width.to_bytes(4, "big")
        + height.to_bytes(4, "big")
        + b"\x08\x02\x00\x00\x00"
    )


def gif(width, height):
    return b"GIF89a" + struct.pack("<HH", width, height) + b"\x00\x00\x00"


def jpeg(width, height):
    return (
        b"\xff\xd8\xff\xe0\x00\x0dJFIF\x00\x00\x00\x00\x00\x00\x00"
        + b"\xff\xc0\x00\x11\x08"
        + height.to_bytes(2, "big")
        + width.to_bytes(2, "big")
        + b"\x03\x01\x11\x00\x02\x11\x00\x03\x11\x00\xff\xd9"
    )


def progressive_jpeg(width, height):
    return (
        b"\xff\xd8\xff\xc2\x00\x11\x08"
        + height.to_bytes(2, "big")
        + width.to_bytes(2, "big")
        + b"\x03\x01\x11\x00\x02\x11\x00\x03\x11\x00\xff\xd9"
    )


def jpeg_with_app_segment(width, height):
    app = b"\xff\xe1\x00\x10Exif\x00\x00" + b"\x00" * 8
    sof = (
        b"\xff\xc0\x00\x11\x08"
        + height.to_bytes(2, "big")
        + width.to_bytes(2, "big")
        + b"\x03\x01\x11\x00\x02\x11\x00\x03\x11\x00"
    )
    return b"\xff\xd8" + app + sof + b"\xff\xd9"


def webp_vp8x(width, height):
    return (
        b"RIFF"
        + (22).to_bytes(4, "little")
        + b"WEBPVP8X"
        + (10).to_bytes(4, "little")
        + b"\x00\x00\x00\x00"
        + (width - 1).to_bytes(3, "little")
        + (height - 1).to_bytes(3, "little")
    )


def webp_vp8l(width, height):
    bits = ((width - 1) & 0x3FFF) | (((height - 1) & 0x3FFF) << 14)
    payload = b"\x2f" + bits.to_bytes(4, "little") + b"\x00" * 5
    return b"RIFF" + (22).to_bytes(4, "little") + b"WEBPVP8L" + len(payload).to_bytes(4, "little") + payload


def normalize_type(value):
    return "jpeg" if value in {"jpg", "jpeg"} else value


def rows_from_stdout(stdout):
    parsed = json.loads(stdout) if stdout.strip() else []
    if isinstance(parsed, dict) and isinstance(parsed.get("results"), list):
        parsed = parsed["results"]
    if not isinstance(parsed, list):
        raise TypeError("json output must be a list or an object with a results list")
    return parsed


try:
    with tempfile.TemporaryDirectory() as td:
        folder = Path(td)
        files = []
        fixtures = [
            ("a.png", png(17, 9)),
            ("b.gif", gif(5, 6)),
            ("c.jpg", jpeg(31, 22)),
            ("d.webp", webp_vp8x(40, 30)),
            ("e-progressive.jpg", progressive_jpeg(33, 44)),
            ("f-app.jpg", jpeg_with_app_segment(45, 46)),
        ]
        for name, content in fixtures:
            path = folder / name
            path.write_bytes(content)
            files.append(path)

        cp = run_cmd([sys.executable, "main.py", *[str(p) for p in files], "--json"])
        record("cli exits successfully", cp.returncode == 0, cp.stderr or cp.stdout)
        try:
            rows = rows_from_stdout(cp.stdout)
        except Exception as exc:
            rows = []
            record("cli json can be parsed", False, f"{type(exc).__name__}: {exc}")
        else:
            record("cli json can be parsed", True)

        def dims_for(name):
            for row in rows:
                if not isinstance(row, dict):
                    continue
                if Path(str(row.get("path", ""))).name == name:
                    return (normalize_type(row.get("type")), row.get("width"), row.get("height"))
            return None

        safe_record("cli includes one row per image", lambda: len([r for r in rows if isinstance(r, dict)]) >= len(fixtures))
        safe_record("cli detects png dimensions", lambda: dims_for("a.png") == ("png", 17, 9))
        safe_record("cli detects gif dimensions", lambda: dims_for("b.gif") == ("gif", 5, 6))
        safe_record("cli detects jpeg dimensions", lambda: dims_for("c.jpg") == ("jpeg", 31, 22))
        safe_record("cli detects progressive jpeg dimensions", lambda: dims_for("e-progressive.jpg") == ("jpeg", 33, 44))
        safe_record("cli scans jpeg markers past app segments", lambda: dims_for("f-app.jpg") == ("jpeg", 45, 46))
        safe_record("cli detects webp vp8x dimensions", lambda: dims_for("d.webp") == ("webp", 40, 30))

        sys.path.insert(0, str(TARGET))
        import main as module

        safe_record("api accepts png bytes input", lambda: module.get_size(png(3, 4)) == {"type": "png", "width": 3, "height": 4})
        safe_record("api accepts gif bytes input", lambda: module.get_size(gif(7, 8)) == {"type": "gif", "width": 7, "height": 8})
        safe_record("api accepts jpeg bytes input", lambda: normalize_type(module.get_size(jpeg(9, 10)).get("type")) == "jpeg" and module.get_size(jpeg(9, 10)).get("width") == 9 and module.get_size(jpeg(9, 10)).get("height") == 10)
        safe_record("api accepts webp vp8x bytes input", lambda: module.get_size(webp_vp8x(11, 12)) == {"type": "webp", "width": 11, "height": 12})
        safe_record("api accepts webp vp8l bytes input", lambda: module.get_size(webp_vp8l(13, 14)) == {"type": "webp", "width": 13, "height": 14})
        safe_record("api accepts filesystem path input", lambda: module.get_size(str(files[0])) == {"type": "png", "width": 17, "height": 9})
        safe_record("api accepts pathlib path input", lambda: module.get_size(files[1]) == {"type": "gif", "width": 5, "height": 6})
        safe_record("api accepts bytearray input", lambda: module.get_size(bytearray(png(15, 16))) == {"type": "png", "width": 15, "height": 16})

        def corrupt_raises():
            try:
                module.get_size(b"not image")
            except Exception:
                return True
            return False

        safe_record("corrupt input raises an error", corrupt_raises)

        bad = folder / "bad.bin"
        bad.write_bytes(b"not image")
        cp_bad = run_cmd([sys.executable, "main.py", str(bad), "--json"])
        record("cli reports unsupported files as failure", cp_bad.returncode != 0 or "error" in (cp_bad.stdout + cp_bad.stderr).lower(), cp_bad.stderr or cp_bad.stdout)
finally:
    write_reward()
