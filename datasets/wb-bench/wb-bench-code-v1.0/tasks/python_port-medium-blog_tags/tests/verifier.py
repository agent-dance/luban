import json
import os
import re
import subprocess
import sys
import tempfile
from html.parser import HTMLParser
from pathlib import Path

sys.dont_write_bytecode = True

ROOT = Path(os.environ.get("WORKSPACE", "/workspace"))
TARGET = ROOT / "target_python"
LOG_DIR = Path(os.environ.get("LOG_DIR", "/logs/verifier"))
LOG_DIR.mkdir(parents=True, exist_ok=True)
RESULTS = []


class TextCollector(HTMLParser):
    def __init__(self):
        super().__init__()
        self.text = []
        self.links = []

    def handle_data(self, data):
        if data.strip():
            self.text.append(data.strip())

    def handle_starttag(self, tag, attrs):
        if tag == "a":
            self.links.append(dict(attrs).get("href", ""))


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


def text_and_links(html):
    parser = TextCollector()
    parser.feed(html)
    return " ".join(parser.text), parser.links


def links_to_slug(links, slug):
    normalized = [href.split("#", 1)[0].split("?", 1)[0].replace("\\", "/") for href in links]
    return any(
        href.endswith(f"posts/{slug}/") or href.rstrip("/").endswith(f"posts/{slug}") or href.endswith(f"posts/{slug}/index.html")
        for href in normalized
    )


try:
    with tempfile.TemporaryDirectory() as td:
        root = Path(td) / "blog"
        (root / "content" / "posts").mkdir(parents=True)
        (root / "templates").mkdir()
        (root / "config.toml").write_text('title = "Kitchen Notes"\n')
        (root / "templates" / "post.html").write_text(
            "<html><h1>{{ title }}</h1><article>{{ content }}</article><p>{{ tags }}</p><small>{{ date }}</small></html>"
        )
        (root / "templates" / "index.html").write_text("<h1>{{ site_title }}</h1><ul>{{ posts }}</ul>")
        (root / "templates" / "tag.html").write_text("<h1>Tag: {{ tag }}</h1><ul>{{ posts }}</ul>")
        (root / "content" / "posts" / "tea.md").write_text(
            '+++\ntitle = "Tea"\ndate = "2024-01-05"\n[taxonomies]\ntags = ["drink", "home"]\n+++\n# Tea\n\nWarm notes.'
        )
        (root / "content" / "posts" / "bread.md").write_text(
            '+++\ntitle = "Bread"\ndate = "2024-02-10"\n[taxonomies]\ntags = ["food", "home"]\n+++\n# Bread\n\nCrusty.'
        )
        (root / "content" / "posts" / "jam.md").write_text(
            '+++\ntitle = "Jam"\ndate = "2023-11-20"\n[taxonomies]\ntags = ["food"]\n+++\n# Jam\n\nSweet.'
        )
        out = Path(td) / "public"
        cp = run_cmd([sys.executable, "main.py", "build", "--root", str(root), "--output", str(out)])
        record("build command exits successfully", cp.returncode == 0, cp.stderr or cp.stdout)

        tea = out / "posts" / "tea" / "index.html"
        bread = out / "posts" / "bread" / "index.html"
        jam = out / "posts" / "jam" / "index.html"
        idx = out / "index.html"
        home = out / "tags" / "home" / "index.html"
        drink = out / "tags" / "drink" / "index.html"
        food = out / "tags" / "food" / "index.html"

        bread_html = bread.read_text() if bread.exists() else ""
        tea_html = tea.read_text() if tea.exists() else ""
        idx_html = idx.read_text() if idx.exists() else ""
        home_html = home.read_text() if home.exists() else ""
        drink_html = drink.read_text() if drink.exists() else ""
        food_html = food.read_text() if food.exists() else ""
        bread_text, _ = text_and_links(bread_html)
        tea_text, _ = text_and_links(tea_html)
        idx_text, idx_links = text_and_links(idx_html)
        home_text, _ = text_and_links(home_html)
        drink_text, _ = text_and_links(drink_html)
        food_text, food_links = text_and_links(food_html)

        safe_record("all post pages are generated", lambda: tea.exists() and bread.exists() and jam.exists())
        safe_record("index page is generated", lambda: idx.exists())
        safe_record("tag pages are generated", lambda: home.exists() and drink.exists() and food.exists())
        safe_record("post template receives title", lambda: "Bread" in bread_text and re.search(r"<h1[^>]*>\s*Bread\s*</h1>", bread_html) is not None)
        safe_record("post template receives markdown content", lambda: "Crusty." in bread_text and re.search(r"<h1[^>]*>\s*Bread\s*</h1>", bread_html) is not None)
        safe_record("post template receives tags", lambda: "food" in bread_text and "home" in bread_text)
        safe_record("post template receives date", lambda: "2024-02-10" in bread_text)
        safe_record("markdown content renders heading for another post", lambda: re.search(r"<h1[^>]*>\s*Tea\s*</h1>", tea_html) is not None and "Warm notes." in tea_text)
        safe_record("post template placeholders are replaced", lambda: "{{" not in bread_html and "}}" not in bread_html)
        safe_record("index uses site title", lambda: "Kitchen Notes" in idx_text)
        safe_record("index includes all posts", lambda: all(title in idx_text for title in ["Bread", "Tea", "Jam"]))
        safe_record("index lists posts newest first", lambda: idx_html.find("Bread") != -1 and idx_html.find("Bread") < idx_html.find("Tea") < idx_html.find("Jam"))
        safe_record("index template placeholders are replaced", lambda: "{{" not in idx_html and "}}" not in idx_html)
        safe_record("index links to post pages", lambda: links_to_slug(idx_links, "bread") and links_to_slug(idx_links, "tea"))
        safe_record("home tag groups matching posts", lambda: "Bread" in home_text and "Tea" in home_text and "Jam" not in home_text)
        safe_record("drink tag only includes tea", lambda: "Tea" in drink_text and "Bread" not in drink_text and "Jam" not in drink_text)
        safe_record("food tag includes food posts", lambda: "Bread" in food_text and "Jam" in food_text and "Tea" not in food_text)
        safe_record("tag template placeholders are replaced", lambda: "{{" not in food_html and "}}" not in food_html)
        safe_record("tag page links to post pages", lambda: links_to_slug(food_links, "bread") and links_to_slug(food_links, "jam"))
finally:
    write_reward()
