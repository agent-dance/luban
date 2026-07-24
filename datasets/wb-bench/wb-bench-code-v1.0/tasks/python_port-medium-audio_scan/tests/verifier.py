import json
import os
import struct
import subprocess
import sys
import tempfile
import wave
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


def make_wav(path, channels=2, rate=8000, frames=800):
    with wave.open(str(path), "wb") as wav:
        wav.setnchannels(channels)
        wav.setsampwidth(2)
        wav.setframerate(rate)
        wav.writeframes(struct.pack("<" + "h" * frames * channels, *([0] * frames * channels)))


def make_id3v1_mp3(path):
    body = b"\xff\xfb" + b"\x00" * 64
    tag = (
        b"TAG"
        + b"Blue Song".ljust(30, b"\x00")
        + b"Ada".ljust(30, b"\x00")
        + b"Home Album".ljust(30, b"\x00")
        + b"2024"
        + b"comment".ljust(30, b"\x00")
        + b"\x00"
    )
    path.write_bytes(body + tag)


def syncsafe(n):
    return bytes([(n >> 21) & 0x7F, (n >> 14) & 0x7F, (n >> 7) & 0x7F, n & 0x7F])


def text_frame(frame_id, value):
    payload = b"\x00" + value.encode("latin1")
    return frame_id.encode("ascii") + len(payload).to_bytes(4, "big") + b"\x00\x00" + payload


def make_id3v2_mp3(path):
    frames = text_frame("TIT2", "Red Song") + text_frame("TPE1", "Bea") + text_frame("TALB", "Road Album")
    header = b"ID3" + b"\x03\x00\x00" + syncsafe(len(frames))
    path.write_bytes(header + frames + b"\xff\xfb" + b"\x00" * 32)


def make_tagless_mp3(path):
    path.write_bytes(b"\xff\xfb" + b"\x00" * 96)


try:
    with tempfile.TemporaryDirectory() as td:
        audio = Path(td) / "audio"
        audio.mkdir()
        out = Path(td) / "metadata.json"
        make_wav(audio / "tone.wav", channels=2, rate=8000, frames=800)
        make_wav(audio / "ambient.wav", channels=1, rate=16000, frames=1600)
        make_wav(audio / "LOUD.WAV", channels=1, rate=11025, frames=1103)
        make_id3v1_mp3(audio / "song.mp3")
        make_id3v1_mp3(audio / "UPPER.MP3")
        make_id3v2_mp3(audio / "tagged.mp3")
        make_tagless_mp3(audio / "plain.mp3")
        (audio / "ignore.txt").write_text("not audio")

        cp = run_cmd([sys.executable, "main.py", "scan", str(audio), "--output", str(out)])
        record("scan command exits successfully", cp.returncode == 0, cp.stderr or cp.stdout)
        try:
            records = json.loads(out.read_text()) if out.exists() else []
        except Exception as exc:
            records = []
            record("metadata output is valid json", False, f"{type(exc).__name__}: {exc}")
        else:
            record("metadata output is valid json", isinstance(records, list))

        byname = {Path(item.get("path", "")).name: item for item in records if isinstance(item, dict)}
        safe_record(
            "outputs one item per supported audio file",
            lambda: set(byname) == {"ambient.wav", "tone.wav", "LOUD.WAV", "song.mp3", "UPPER.MP3", "tagged.mp3", "plain.mp3"},
        )
        safe_record("ignores unsupported files", lambda: "ignore.txt" not in byname)
        safe_record("output is sorted by path", lambda: [Path(item.get("path", "")).name for item in records] == sorted(Path(item.get("path", "")).name for item in records))

        tone = byname.get("tone.wav", {})
        ambient = byname.get("ambient.wav", {})
        loud = byname.get("LOUD.WAV", {})
        song = byname.get("song.mp3", {})
        upper = byname.get("UPPER.MP3", {})
        tagged = byname.get("tagged.mp3", {})
        plain = byname.get("plain.mp3", {})

        safe_record("wav format field is present", lambda: tone.get("format") == "wav" and ambient.get("format") == "wav")
        safe_record("wav sample rate is parsed", lambda: tone.get("sample_rate") == 8000 and ambient.get("sample_rate") == 16000)
        safe_record("wav channel count is parsed", lambda: tone.get("channels") == 2 and ambient.get("channels") == 1)
        safe_record("wav duration is calculated", lambda: abs(tone.get("duration", 0) - 0.1) < 0.01 and abs(ambient.get("duration", 0) - 0.1) < 0.01)
        safe_record("missing wav tags are null", lambda: tone.get("title") is None and tone.get("artist") is None and tone.get("album") is None)
        safe_record("uppercase wav extension is scanned", lambda: loud.get("format") == "wav" and loud.get("sample_rate") == 11025)

        safe_record("mp3 id3v1 format field is present", lambda: song.get("format") == "mp3")
        safe_record("mp3 id3v1 title is parsed", lambda: song.get("title") == "Blue Song")
        safe_record("mp3 id3v1 artist and album are parsed", lambda: song.get("artist") == "Ada" and song.get("album") == "Home Album")
        safe_record("uppercase mp3 extension is scanned", lambda: upper.get("format") == "mp3" and upper.get("title") == "Blue Song")
        safe_record("tagless mp3 keeps missing tags null", lambda: plain.get("format") == "mp3" and plain.get("title") is None and plain.get("artist") is None and plain.get("album") is None)
        safe_record("mp3 id3v2 format field is present", lambda: tagged.get("format") == "mp3")
        safe_record("mp3 id3v2 title is parsed", lambda: tagged.get("title") == "Red Song")
        safe_record("mp3 id3v2 artist and album are parsed", lambda: tagged.get("artist") == "Bea" and tagged.get("album") == "Road Album")
finally:
    write_reward()
