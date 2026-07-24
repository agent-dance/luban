#!/bin/bash
set -euo pipefail
if [ -d /workspace ]; then cd /workspace; else cd "$(pwd)"; fi
cat > target_python/main.py <<'PY_MAIN'
#!/usr/bin/env python3
import argparse, json, struct
from pathlib import Path

def read_bytes(path_or_bytes):
    if isinstance(path_or_bytes, (bytes, bytearray)):
        return bytes(path_or_bytes)
    return Path(path_or_bytes).read_bytes()

def get_size(path_or_bytes):
    data = read_bytes(path_or_bytes)
    if data.startswith(b'\x89PNG\r\n\x1a\n') and len(data) >= 24:
        width, height = struct.unpack('>II', data[16:24])
        return {'type': 'png', 'width': width, 'height': height}
    if data[:6] in (b'GIF87a', b'GIF89a') and len(data) >= 10:
        width, height = struct.unpack('<HH', data[6:10])
        return {'type': 'gif', 'width': width, 'height': height}
    if data.startswith(b'\xff\xd8'):
        i = 2
        while i + 9 < len(data):
            while i < len(data) and data[i] != 0xff:
                i += 1
            while i < len(data) and data[i] == 0xff:
                i += 1
            if i >= len(data):
                break
            marker = data[i]
            i += 1
            if marker in (0xd8, 0xd9):
                continue
            if i + 2 > len(data):
                break
            size = int.from_bytes(data[i:i+2], 'big')
            if size < 2:
                break
            if marker in set(range(0xc0, 0xc4)) | set(range(0xc5, 0xc8)) | set(range(0xc9, 0xcc)) | set(range(0xcd, 0xd0)):
                if i + 7 <= len(data):
                    height = int.from_bytes(data[i+3:i+5], 'big')
                    width = int.from_bytes(data[i+5:i+7], 'big')
                    return {'type': 'jpeg', 'width': width, 'height': height}
            i += size
    if data.startswith(b'RIFF') and len(data) >= 30 and data[8:12] == b'WEBP':
        chunk = data[12:16]
        if chunk == b'VP8X' and len(data) >= 30:
            width = 1 + int.from_bytes(data[24:27], 'little')
            height = 1 + int.from_bytes(data[27:30], 'little')
            return {'type': 'webp', 'width': width, 'height': height}
        if chunk == b'VP8L' and len(data) >= 25 and data[20] == 0x2f:
            bits = int.from_bytes(data[21:25], 'little')
            width = (bits & 0x3fff) + 1
            height = ((bits >> 14) & 0x3fff) + 1
            return {'type': 'webp', 'width': width, 'height': height}
    raise ValueError('unsupported or corrupt image')

def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument('images', nargs='+')
    parser.add_argument('--json', action='store_true')
    args = parser.parse_args(argv)
    rows = []
    for image in args.images:
        row = get_size(image)
        row['path'] = image
        rows.append(row)
    if args.json:
        print(json.dumps(rows, indent=2))
    else:
        for row in rows:
            print(f'{row["path"]}: {row["width"]}x{row["height"]} {row["type"]}')

if __name__ == '__main__':
    main()
PY_MAIN
chmod +x target_python/main.py
