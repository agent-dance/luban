#!/bin/bash
set -euo pipefail
if [ -d /workspace ]; then cd /workspace; else cd "$(pwd)"; fi
cat > target_python/main.py <<'PY_MAIN'
#!/usr/bin/env python3
import argparse, re, xml.etree.ElementTree as ET
from pathlib import Path

ET.register_namespace('', 'http://www.w3.org/2000/svg')

def strip_ns(tag):
    return tag.rsplit('}', 1)[-1] if '}' in tag else tag

def clean_element(element):
    for child in list(element):
        clean_element(child)
        name = strip_ns(child.tag)
        empty = not list(child) and not (child.text or '').strip()
        if name in {'metadata', 'title'} or (name == 'g' and empty):
            element.remove(child)
    for key, value in list(element.attrib.items()):
        local = key.rsplit('}', 1)[-1] if '}' in key else key
        if (local == 'fill' and value == 'none' and strip_ns(element.tag) == 'g') or (local == 'stroke' and value == 'none'):
            del element.attrib[key]

def optimize_text(text):
    text = re.sub(r'<!--.*?-->', '', text, flags=re.S)
    root = ET.fromstring(text)
    clean_element(root)
    return ET.tostring(root, encoding='unicode')

def convert(input_path, output_path):
    src, dst = Path(input_path), Path(output_path)
    if src.is_dir():
        dst.mkdir(parents=True, exist_ok=True)
        for item in sorted(src.glob('*.svg')):
            (dst / item.name).write_text(optimize_text(item.read_text()))
    else:
        dst.parent.mkdir(parents=True, exist_ok=True)
        dst.write_text(optimize_text(src.read_text()))

def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument('input')
    parser.add_argument('-o', '--output', required=True)
    args = parser.parse_args(argv)
    convert(args.input, args.output)

if __name__ == '__main__':
    main()
PY_MAIN
chmod +x target_python/main.py
