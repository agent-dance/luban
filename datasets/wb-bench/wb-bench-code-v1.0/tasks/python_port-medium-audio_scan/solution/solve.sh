#!/bin/bash
set -euo pipefail
if [ -d /workspace ]; then cd /workspace; else cd "$(pwd)"; fi
cat > target_python/main.py <<'PY_MAIN'
#!/usr/bin/env python3
import argparse, json, wave
from pathlib import Path

def clean(raw):
    text = raw.split(b'\x00', 1)[0].decode('latin1', errors='ignore').strip()
    return text or None

def read_wav(path):
    with wave.open(str(path), 'rb') as wav:
        frames = wav.getnframes()
        rate = wav.getframerate()
        channels = wav.getnchannels()
    return {'path': str(path), 'format': 'wav', 'duration': frames / rate if rate else None, 'sample_rate': rate, 'channels': channels, 'title': None, 'artist': None, 'album': None}

def syncsafe(raw):
    return ((raw[0] & 0x7f) << 21) | ((raw[1] & 0x7f) << 14) | ((raw[2] & 0x7f) << 7) | (raw[3] & 0x7f)

def read_id3v2(data):
    tags = {}
    if len(data) >= 10 and data[:3] == b'ID3':
        size = syncsafe(data[6:10])
        pos, end = 10, min(len(data), 10 + size)
        while pos + 10 <= end:
            frame_id = data[pos:pos+4].decode('latin1', errors='ignore')
            frame_size = int.from_bytes(data[pos+4:pos+8], 'big')
            pos += 10
            if not frame_id.strip('\x00') or frame_size <= 0:
                break
            frame = data[pos:pos+frame_size]
            pos += frame_size
            if frame_id in {'TIT2', 'TPE1', 'TALB'} and frame:
                raw = frame[1:]
                value = raw.decode('utf-16', errors='ignore') if frame[0] in (1, 2) else raw.decode('latin1', errors='ignore')
                tags[{'TIT2': 'title', 'TPE1': 'artist', 'TALB': 'album'}[frame_id]] = value.strip('\x00').strip() or None
    return tags

def read_id3v1(data):
    if len(data) >= 128 and data[-128:-125] == b'TAG':
        return {'title': clean(data[-125:-95]), 'artist': clean(data[-95:-65]), 'album': clean(data[-65:-35])}
    return {}

def read_mp3(path):
    data = Path(path).read_bytes()
    tags = {'title': None, 'artist': None, 'album': None}
    tags.update({k: v for k, v in read_id3v1(data).items() if v})
    tags.update({k: v for k, v in read_id3v2(data).items() if v})
    item = {'path': str(path), 'format': 'mp3', 'duration': None, 'sample_rate': None, 'channels': None}
    item.update(tags)
    return item

def scan_dir(audio_dir):
    rows = []
    for path in sorted(Path(audio_dir).iterdir(), key=lambda p: p.name):
        if path.suffix.lower() == '.wav':
            rows.append(read_wav(path))
        elif path.suffix.lower() == '.mp3':
            rows.append(read_mp3(path))
    return rows

def main(argv=None):
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest='command')
    scan = sub.add_parser('scan')
    scan.add_argument('audio_dir')
    scan.add_argument('--output', required=True)
    args = parser.parse_args(argv)
    if args.command != 'scan':
        parser.error('expected scan')
    Path(args.output).write_text(json.dumps(scan_dir(args.audio_dir), indent=2))

if __name__ == '__main__':
    main()
PY_MAIN
chmod +x target_python/main.py
