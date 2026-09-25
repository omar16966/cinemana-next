# -*- coding: utf-8 -*-
# تنزيل خطوط عربية من Google Fonts ودمجها محلياً في التطبيق
# (المزايا: تعمل بلا إنترنت، ولا اعتماد على CDN وقت التشغيل)
# ينتج: desktop/frontend/src/assets/fonts/*.woff2 + fonts.css
import re
import os
import urllib.request

UA = ("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/124.0 Safari/537.36")

# العائلات المطلوبة مع الوزن المناسب للترجمات (وضوح فوق الفيديو)
FAMILIES = [
    ("Cairo", "family=Cairo:wght@700"),
    ("Tajawal", "family=Tajawal:wght@700"),
    ("Almarai", "family=Almarai:wght@700"),
    ("Amiri", "family=Amiri"),
    ("Noto Kufi Arabic", "family=Noto+Kufi+Arabic:wght@600"),
    ("Aref Ruqaa", "family=Aref+Ruqaa"),
    ("Lalezar", "family=Lalezar"),
]

OUT = os.path.join(os.path.dirname(__file__), "frontend", "src", "assets", "fonts")
os.makedirs(OUT, exist_ok=True)

def fetch(url):
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=30) as r:
        return r.read()

css_out = []
downloaded = 0
for display, query in FAMILIES:
    css = fetch("https://fonts.googleapis.com/css2?%s&display=swap" % query).decode("utf-8")
    # تقسيم إلى كتل @font-face مع تعليق المجموعة (/* arabic */ ...)
    blocks = re.findall(r"/\*\s*([a-z0-9-]+)\s*\*/\s*@font-face\s*\{(.*?)\}", css, re.S)
    for subset, body in blocks:
        if subset not in ("arabic", "latin", "latin-ext"):
            continue
        m_url = re.search(r"src:\s*url\((https://[^)]+\.woff2)\)", body)
        m_fam = re.search(r"font-family:\s*'([^']+)'", body)
        m_wgt = re.search(r"font-weight:\s*(\d+)", body)
        m_rng = re.search(r"unicode-range:\s*([^;]+);", body)
        if not (m_url and m_fam and m_rng):
            continue
        family = m_fam.group(1)
        weight = m_wgt.group(1) if m_wgt else "400"
        slug = family.lower().replace(" ", "-")
        fname = "%s-%s.woff2" % (slug, subset)
        path = os.path.join(OUT, fname)
        if not os.path.exists(path):
            data = fetch(m_url.group(1))
            with open(path, "wb") as f:
                f.write(data)
            downloaded += 1
        css_out.append(
            "@font-face {\n"
            "  font-family: '%s';\n"
            "  font-style: normal;\n"
            "  font-weight: %s;\n"
            "  font-display: swap;\n"
            "  src: url('./%s') format('woff2');\n"
            "  unicode-range: %s;\n"
            "}" % (family, weight, fname, m_rng.group(1).strip())
        )
    print("done:", family)

with open(os.path.join(OUT, "fonts.css"), "w", encoding="utf-8") as f:
    f.write("/* خطوط عربية مدمجة محلياً (OFL - Google Fonts) — تُنتج بـ download_fonts.py */\n")
    f.write("\n".join(css_out) + "\n")
print("fonts downloaded:", downloaded, "| @font-face rules:", len(css_out))
