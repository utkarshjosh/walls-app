#!/usr/bin/env python3
"""scenekit - make live wallpaper scenes without writing the plumbing.

A scene is one self-contained HTML file the app draws behind the desktop icons, plus a
poster and a small description. The rules a scene must meet are in the live scene
standard (docs/16-live-scene-standard.md). This kit adds everything those rules ask for
around the part that actually draws: the security policy, the geometry that lets one
picture span several displays, wall-clock time, the 30 frames-a-second cap, the reduced
render size, pause, stopping when hidden, and the fallback when WebGL is missing.

Where things live
  scene-sources/<slug>/     the source of one scene: scene.json (title, author, licence,
                            settings) and either shader.glsl or scene.html
  scene-sources/runtime/    the shared runtime - one copy, built into every scene
  scenes/<slug>/            the finished scene: scene.html, meta.json, poster.jpg.
                            Never edit these by hand; they are rewritten by this kit.

Typical use
  You found a shader on the web (and its licence allows commercial redistribution):
      python3 scenekit.py wrap-shader glow.glsl --slug soft-glow --title "Soft Glow" \\
          --author "Jane Doe" --license "CC BY 4.0" --source https://example.org/glow
      python3 scenekit.py poster soft-glow

  You want a clock, a calendar, anything made of ordinary HTML and CSS:
      python3 scenekit.py new-dom --slug desk-calendar --title "Desk Calendar"
      ... edit scene-sources/desk-calendar/scene.html ...
      python3 scenekit.py build desk-calendar
      python3 scenekit.py poster desk-calendar --t 36540

  You changed a source or the runtime:
      python3 scenekit.py build            (everything)
      python3 scenekit.py build tidal lava (just these)

Every scene then still has to pass the gate (`walls scene check <folder>`) and be watched
by a person before it is published. Python 3 standard library only.
"""
import argparse
import html as htmllib
import json
import os
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import urllib.parse
import zlib

HERE = os.path.dirname(os.path.abspath(__file__))
RUNTIME = os.path.join(HERE, "runtime")
OUT = os.path.normpath(os.path.join(HERE, "..", "scenes"))   # where `walls seed` looks for scenes

STANDARD = 1
MAX_HTML_BYTES = 512 * 1024
MAX_SIDE = 1600            # the standard's limit for a shader's render buffer
POSTER_W, POSTER_H = 1920, 1200

# The one policy line of the standard. It goes directly after the charset line, and the
# text "Content-Security-Policy" may appear nowhere else in a scene.
POLICY = ("<meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; script-src 'unsafe-inline'; "
          "style-src 'unsafe-inline'; img-src data:; font-src data:; base-uri 'none'; form-action 'none'\">")
OPENING = re.compile(r"\A\s*<!doctype html>\s*<html\b[^>]*>\s*<head>\s*<meta charset=\"utf-8\">\s*" + re.escape(POLICY), re.I)

SLUG = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
HEX = re.compile(r"^#[0-9a-fA-F]{6}$")
SAFE_CSS = re.compile(r"^[#A-Za-z0-9(),.% -]+$")

DEFAULT_COLORS = ["#0c0d12", "#14161d", "#1b1d26"]


class Problem(Exception):
    """Something the person running the kit has to put right."""


def read(path):
    with open(path, encoding="utf-8") as f:
        return f.read()


def write(path, text):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write(text)


def runtime(name):
    return read(os.path.join(RUNTIME, name))


def say(*parts):
    print(*parts)


# --------------------------------------------------------------------------------------
# The rules of the standard, checked on every scene before it is written.
# --------------------------------------------------------------------------------------

# Certain violations: (what it is, pattern). Tags are matched without regard to case.
FORBIDDEN = [
    ("<script src>", r"(?i)<script\b[^>]*\bsrc\s*="),
    ("<link>", r"(?i)<link\b"), ("<iframe>", r"(?i)<iframe\b"), ("<object>", r"(?i)<object\b"),
    ("<embed>", r"(?i)<embed\b"), ("<form>", r"(?i)<form\b"), ("<base>", r"(?i)<base\b"),
    ("<a href>", r"(?i)<a\s[^>]*\bhref\b"),
    ("a network address (http:, https:, ws:)", r"\b(?:https?|wss?):"),
    ("fetch", r"\bfetch\b"), ("XMLHttpRequest", r"XMLHttpRequest"), ("WebSocket", r"WebSocket"),
    ("EventSource", r"EventSource"), ("sendBeacon", r"sendBeacon"), ("importScripts", r"importScripts"),
    ("dynamic import(", r"\bimport\s*\("), ("eval", r"\beval\b"), ("new Function", r"\bnew\s+Function\b"),
    ("localStorage", r"localStorage"), ("sessionStorage", r"sessionStorage"), ("indexedDB", r"indexedDB"),
    ("document.cookie", r"\bdocument\s*\.\s*cookie\b"), ("window.open", r"\bwindow\s*\.\s*open\b"),
    ("an assignment to location", r"\blocation\b(?:\s*\.\s*\w+)?\s*=(?!=)|\blocation\s*\.\s*(?:assign|replace|reload)\b"),
    ("a navigator API other than hardwareConcurrency and userAgent", r"\bnavigator\s*\.\s*(?!hardwareConcurrency\b|userAgent\b)"),
    ("Notification", r"\bNotification\b"), ("postMessage", r"postMessage"),
    ("top. or parent. access", r"(?<![\w$.])(?:top|parent)\s*\."),
]
# Plain fragments a cruder checker could trip over even where they are harmless
# (a comment that ends "...the desktop." contains "top."). Reported as warnings.
RISKY = ["top.", "parent.", "eval", "fetch", "http:", "https:", "ws:", "import(", "navigator."]


def line_of(text, pos):
    return text.count("\n", 0, pos) + 1


def lint(text):
    """Check a finished scene.html against the standard. Returns (errors, warnings)."""
    errors, warnings = [], []
    if len(text.encode("utf-8")) > MAX_HTML_BYTES:
        errors.append("the file is larger than 512 KB")
    opening = OPENING.match(text)
    if not opening:
        errors.append("the file must open with the doctype, <html>, <head>, the charset line and then the policy line, "
                      "with nothing else in between")
    elif not opening.group(0).isascii():
        errors.append("everything up to the end of the policy line must be plain ASCII")
    if len(re.findall(r"(?i)content-security-policy", text)) != 1:
        errors.append("the words Content-Security-Policy must appear exactly once (the policy line itself); "
                      "describe the policy in words elsewhere")
    if "location.hash" not in text or "paused" not in text:
        errors.append("the scene must read location.hash and handle paused")
    seen = set()
    for what, pattern in FORBIDDEN:
        for m in re.finditer(pattern, text):
            errors.append("line %d: %s is not allowed in a scene (%r)" % (line_of(text, m.start()), what, m.group(0)[:40]))
            seen.add(line_of(text, m.start()))
            break
    for frag in RISKY:
        start = 0
        while True:
            i = text.find(frag, start)
            if i < 0:
                break
            start = i + len(frag)
            allowed = frag == "navigator." and re.match(r"navigator\.(hardwareConcurrency|userAgent)\b", text[i:])
            if line_of(text, i) not in seen and not allowed:
                snippet = text[max(0, i - 18):i + len(frag) + 8].replace("\n", " ")
                warnings.append("line %d: contains %r (...%s...); harmless here, but a strict checker may refuse it - reword it"
                                % (line_of(text, i), frag, snippet))
                break
    return errors, warnings


# --------------------------------------------------------------------------------------
# Sources
# --------------------------------------------------------------------------------------

def source_dir(slug):
    return os.path.join(HERE, slug)


def all_slugs():
    return sorted(d for d in os.listdir(HERE)
                  if os.path.isfile(os.path.join(HERE, d, "scene.json")))


def load(slug):
    path = os.path.join(source_dir(slug), "scene.json")
    if not os.path.isfile(path):
        raise Problem("there is no scene source called %r (looked for %s)" % (slug, path))
    try:
        cfg = json.loads(read(path))
    except ValueError as e:
        raise Problem("%s is not valid JSON: %s" % (path, e))
    if cfg.get("kind") not in ("shader", "dom"):
        raise Problem('%s: "kind" must be "shader" or "dom"' % path)
    return cfg


def save(slug, cfg):
    write(os.path.join(source_dir(slug), "scene.json"), json.dumps(cfg, indent=2, ensure_ascii=False) + "\n")


def check_slug(slug):
    if not SLUG.match(slug):
        raise Problem("a slug is lowercase letters, digits and single hyphens, like quiet-clock (got %r)" % slug)


def css_value(value, what):
    if not isinstance(value, str) or not SAFE_CSS.match(value) or "url" in value.lower():
        raise Problem("%s must be a plain colour or gradient (got %r)" % (what, value))
    return value


def colours_of(cfg):
    cols = cfg.get("colors") or DEFAULT_COLORS
    for c in cols:
        if not isinstance(c, str) or not HEX.match(c):
            raise Problem('"colors" must be a list of #rrggbb colours, upper to lower (got %r)' % (c,))
    return cols


def darkest(cols):
    def lum(c):
        r, g, b = int(c[1:3], 16), int(c[3:5], 16), int(c[5:7], 16)
        return 0.2126 * r + 0.7152 * g + 0.0722 * b
    return min(cols, key=lum)


def credit(cfg):
    who = (cfg.get("author") or "").strip() or "author not yet given"
    lic = (cfg.get("license") or "").strip() or "licence not yet given"
    return ("By %s. Licence: %s." % (who, lic)).replace("--", "- -").replace("<", "(").replace(">", ")")


def meta_of(cfg):
    cols = colours_of(cfg)
    color = cfg.get("color") or cfg.get("background") or cols[len(cols) // 2]
    if not HEX.match(color):
        raise Problem('"color" must be a #rrggbb colour (got %r)' % (color,))
    return {
        "standard": STANDARD,
        "title": cfg.get("title") or "",
        "tags": cfg.get("tags") or [],
        "color": color,
        "author": cfg.get("author") or "",
        "license": cfg.get("license") or "",
        "sourceUrl": cfg.get("sourceUrl") or None,
        # How the app lays the scene out: one picture across displays, or the
        # whole scene drawn in every window (a clock). The app never stretches
        # a per-display scene, and its preview draws it whole on each display.
        "layout": "per-display" if cfg.get("kind") == "dom" else "canvas",
    }


# --------------------------------------------------------------------------------------
# Shaders written in the common web style
# --------------------------------------------------------------------------------------

SUPPLIED = ("iResolution", "iTime", "iTimeDelta", "iFrameRate", "iFrame", "iMouse", "iDate")
CANNOT_SUPPLY = [
    (r"\biChannel\d*\b|\biChannelResolution\b|\biChannelTime\b", "iChannel inputs (textures, buffers, video, sound or the keyboard)"),
    (r"\b(?:texture|texture2D|textureLod|textureGrad|texelFetch|textureCube|texture2DLod|textureSize)\s*\(", "texture lookups"),
    (r"\bsampler(?:2D|3D|Cube|2DArray)\b", "texture samplers"),
    (r"\biSampleRate\b|\bmainSound\b", "sound"),
    (r"\bmainVR\b|\bmainCubemap\b", "VR or cubemap output"),
]


def strip_glsl_comments(src):
    """Remove // and /* */ comments, keeping every line break so line numbers still match."""
    out, i, n = [], 0, len(src)
    while i < n:
        two = src[i:i + 2]
        if two == "//":
            j = src.find("\n", i)
            i = n if j < 0 else j
        elif two == "/*":
            j = src.find("*/", i + 2)
            j = n if j < 0 else j + 2
            out.append("\n" * src.count("\n", i, j))
            i = j
        else:
            out.append(src[i])
            i += 1
    return "".join(out)


def prepare_web_shader(src, name="the shader"):
    """Check a web-style shader and return (source ready to embed, 'out' or 'inout', notes)."""
    notes = []
    code = strip_glsl_comments(src.replace("\r\n", "\n").replace("\r", "\n"))
    for pattern, what in CANNOT_SUPPLY:
        m = re.search(pattern, code)
        if m:
            raise Problem("%s uses %s (line %d: %s), which the kit cannot supply. A scene may use only time, "
                          "resolution and the date; shaders that need images, buffers, sound or input cannot be wrapped."
                          % (name, what, line_of(code, m.start()), m.group(0).strip()))
    sig = re.search(r"\bvoid\s+mainImage\s*\(\s*(inout|out)\s+vec4\s+\w+\s*,\s*(?:in\s+)?vec2\s+\w+\s*\)", code)
    if not sig:
        raise Problem("%s has no `void mainImage(out vec4 fragColor, in vec2 fragCoord)`. wrap-shader takes shaders "
                      "written in that common web style." % name)
    lines = code.split("\n")
    for k, line in enumerate(lines):
        if re.match(r"\s*#\s*version\b", line):
            lines[k] = ""
            notes.append("line %d: removed the #version line (the kit chooses the version)" % (k + 1))
        elif re.match(r"\s*#\s*extension\b", line):
            lines[k] = ""
            notes.append("line %d: removed an #extension line; under WebGL 2 its features are built in" % (k + 1))
        elif re.match(r"\s*uniform\b", line):
            names = re.findall(r"\b(\w+)\s*(?:\[[^\]]*\])?\s*[,;]", line)
            extra = [x for x in names if x not in SUPPLIED]
            if extra:
                raise Problem("%s declares an input the kit cannot supply (line %d: %s). The kit provides only %s."
                              % (name, k + 1, line.strip(), ", ".join(SUPPLIED)))
            lines[k] = ""
            notes.append("line %d: removed a uniform declaration the kit supplies itself" % (k + 1))
    code = "\n".join(lines)
    for word in ("top", "parent"):                    # harmless names the standard's wording forbids before a dot
        if re.search(r"\b%s\b" % word, code):
            new = word + "_"
            while re.search(r"\b%s\b" % new, code):
                new += "_"
            code = re.sub(r"\b%s\b" % word, new, code)
            notes.append("renamed the identifier %r to %r (the standard forbids the text %r)" % (word, new, word + "."))
    return code.rstrip() + "\n", sig.group(1), notes


# --------------------------------------------------------------------------------------
# Building
# --------------------------------------------------------------------------------------

def fill(template, values):
    for key, value in values.items():
        template = template.replace("{{" + key + "}}", str(value))
    left = re.search(r"\{\{[A-Z0-9]+\}\}", template)
    if left:
        raise Problem("the runtime template has an unknown placeholder %s" % left.group(0))
    return template


def build_shader(slug, cfg):
    path = os.path.join(source_dir(slug), "shader.glsl")
    if not os.path.isfile(path):
        raise Problem("%s is missing" % path)
    shader, notes = read(path), []
    style = cfg.get("style", "web")
    if style == "native":
        # First-party shaders written against the kit's own helpers: `vec3 scene(p, uv, asp, px)`.
        frag = (runtime("shader-lib.glsl") + "\n" + shader + runtime("shader-main.glsl")).strip()
        newer, fade = "false", 0
    elif style == "web":
        body, outq, notes = prepare_web_shader(shader, os.path.relpath(path, HERE))
        frag = runtime("shader-web.glsl").replace("{{OUTQ}}", outq) + body.strip()
        newer, fade = "true", float(cfg.get("fade", 20))
    else:
        raise Problem('%s/scene.json: "style" must be "web" or "native"' % slug)
    if "</script" in frag.lower():
        raise Problem("%s contains the text </script, which cannot go inside a scene" % path)
    loop = float(cfg.get("loop", 900))
    max_side = int(cfg.get("maxSide", MAX_SIDE))
    if not 1 <= loop <= 86400:
        raise Problem('%s: "loop" must be between 1 and 86400 seconds' % slug)
    if not 0 <= fade < loop / 2:
        raise Problem('%s: "fade" must be shorter than half the loop' % slug)
    if not 64 <= max_side <= MAX_SIDE:
        raise Problem('%s: "maxSide" must be between 64 and %d (the standard\'s limit)' % (slug, MAX_SIDE))
    cols = colours_of(cfg)
    background = css_value(cfg.get("background") or darkest(cols), "background")
    fallback = css_value(cfg.get("fallback") or "linear-gradient(180deg,%s)" % ",".join(cols), "fallback")
    num = lambda x: ("%g" % x)
    page = fill(runtime("canvas.html"), {
        "POLICY": POLICY, "STANDARD": STANDARD, "TITLE": htmllib.escape(cfg.get("title") or slug),
        "CREDIT": credit(cfg), "LOOP": num(loop), "FADE": num(fade), "MAXSIDE": max_side, "ES3": newer,
        "BG": background, "FALLBACK": fallback, "FRAG": "\x00FRAG\x00",
    })
    return page.replace("\x00FRAG\x00", frag), notes


def build_dom(slug, cfg):
    path = os.path.join(source_dir(slug), "scene.html")
    if not os.path.isfile(path):
        raise Problem("%s is missing" % path)
    src = read(path)
    for marker in ("<!-- scenekit:policy -->", "<!-- scenekit:runtime -->"):
        if src.count(marker) != 1:
            raise Problem("%s must contain the line %s exactly once" % (path, marker))
    if not re.match(r"\A\s*<!doctype html>\s*<html\b[^>]*>\s*<head>\s*<meta charset=\"utf-8\">\s*<!-- scenekit:policy -->", src, re.I):
        raise Problem("%s must open with the doctype, <html>, <head>, <meta charset=\"utf-8\"> and then the line "
                      "<!-- scenekit:policy -->, with nothing else in between" % path)
    header = ("<!--\n  %s - a live wallpaper scene (per-display scene, live scene standard %d).\n  %s\n"
              "  One self-contained file made by scenekit: it can load nothing and send nothing.\n"
              "  Each display shows the whole scene. The URL fragment carries paused=0|1 and,\n"
              "  for stills, t=<seconds since local midnight>; the geometry values are ignored.\n-->"
              % ((cfg.get("title") or slug).replace("--", "- -"), STANDARD, credit(cfg)))
    page, n = re.subn(r"<!-- scenekit:notes.*?-->", lambda m: header, src, count=1, flags=re.S)
    if n == 0:
        page = page.replace("<title>", header + "\n<title>", 1) if "<title>" in page else page
    page = page.replace("<!-- scenekit:policy -->", POLICY, 1)
    page = page.replace("<!-- scenekit:runtime -->", "<script>\n" + runtime("dom.js").rstrip() + "\n</script>", 1)
    return page, []


def build_one(slug, quiet=False):
    """Write scenes/<slug>/scene.html and meta.json from the source. Returns the scene.html path."""
    check_slug(slug)
    cfg = load(slug)
    page, notes = (build_shader if cfg["kind"] == "shader" else build_dom)(slug, cfg)
    errors, warnings = lint(page)
    if errors:
        raise Problem("%s would not meet the standard, so nothing was written:\n  - %s" % (slug, "\n  - ".join(errors)))
    meta = meta_of(cfg)
    target = os.path.join(OUT, slug)
    write(os.path.join(target, "scene.html"), page)
    write(os.path.join(target, "meta.json"), json.dumps(meta, indent=2, ensure_ascii=False) + "\n")
    if not quiet:
        say("%-14s %6d bytes  %s" % (slug, len(page.encode("utf-8")), os.path.join(target, "scene.html")))
        for note in notes:
            say("    note:", note)
        for w in warnings:
            say("    warning:", w)
        missing = [k for k in ("title", "author", "license") if not meta[k]]
        if missing:
            say("    not publishable yet: scene.json has no %s (the gate refuses a scene without them)" % " or ".join(missing))
        if not os.path.isfile(os.path.join(target, "poster.jpg")):
            say("    no poster yet: python3 scenekit.py poster %s" % slug)
    return os.path.join(target, "scene.html")


# --------------------------------------------------------------------------------------
# A local Chrome: to see whether a shader compiles, and to capture the poster
# --------------------------------------------------------------------------------------

CHROMES = [
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "~/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Google Chrome Beta.app/Contents/MacOS/Google Chrome Beta",
    "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
    "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
]


def find_chrome(explicit=None):
    for c in [explicit, os.environ.get("CHROME")] + CHROMES:
        if c and os.path.isfile(os.path.expanduser(c)) and os.access(os.path.expanduser(c), os.X_OK):
            return os.path.expanduser(c)
    for name in ("google-chrome", "chromium", "chromium-browser", "chrome"):
        found = shutil.which(name)
        if found:
            return found
    return None


def chrome(binary, scene_path, fragment, extra, timeout=90):
    url = "file://" + urllib.parse.quote(os.path.abspath(scene_path)) + "#" + fragment
    with tempfile.TemporaryDirectory(prefix="scenekit-chrome-") as profile:
        cmd = [binary, "--headless=new", "--no-first-run", "--no-default-browser-check", "--disable-extensions",
               "--hide-scrollbars", "--mute-audio", "--force-device-scale-factor=1",
               "--window-size=%d,%d" % (POSTER_W, POSTER_H), "--virtual-time-budget=3000",
               "--user-data-dir=" + profile] + extra + [url]
        try:
            return subprocess.run(cmd, capture_output=True, timeout=timeout)
        except subprocess.TimeoutExpired:
            raise Problem("Chrome did not finish within %d seconds" % timeout)


def probe(binary, scene_path, fragment="t=1"):
    """Load the scene once and report how it drew: ('webgl2'|'webgl'|'still'|None, error text)."""
    done = chrome(binary, scene_path, fragment, ["--dump-dom"])
    dom = done.stdout.decode("utf-8", "replace")
    how = re.search(r'<html[^>]*\bdata-scene="([^"]*)"', dom)
    err = re.search(r'<html[^>]*\bdata-scene-error="([^"]*)"', dom)
    return (how.group(1) if how else None), (htmllib.unescape(err.group(1)) if err else "")


def explain_shader_error(slug, cfg, text):
    """Shift the line numbers in a compiler message so they point into the scene's own shader file."""
    if cfg.get("kind") != "shader" or cfg.get("style", "web") != "web":
        return text
    before = runtime("shader-web.glsl").count("\n") + 2          # the runtime puts two lines in front

    def shift(m):
        line = int(m.group(1))
        return "shader.glsl line %d:" % (line - before) if line > before else "kit adapter line %d:" % line
    return re.sub(r"ERROR: \d+:(\d+):", shift, text)


def report_probe(slug, cfg, binary):
    how, err = probe(binary, os.path.join(OUT, slug, "scene.html"))
    if how in ("webgl2", "webgl"):
        say("    drew in Chrome with %s" % ("WebGL 2" if how == "webgl2" else "WebGL 1"))
        return True
    if how == "still":
        say("    the shader did NOT run in Chrome; the scene shows its fallback gradient instead. Chrome said:")
        seen = []
        for line in explain_shader_error(slug, cfg, err).splitlines():
            if line.strip() and line not in seen:
                seen.append(line)
        for line in seen[:12]:
            say("      " + line)
        return False
    return True                                        # a per-display scene: nothing to report


def read_png(path):
    """A minimal PNG reader (8-bit RGB or RGBA, not interlaced). Returns (width, height, rows of (r, g, b))."""
    data = open(path, "rb").read()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("not a PNG")
    pos, idat, head = 8, b"", None
    while pos < len(data):
        n, kind = struct.unpack(">I4s", data[pos:pos + 8])
        body = data[pos + 8:pos + 8 + n]
        pos += 12 + n
        if kind == b"IHDR":
            head = struct.unpack(">IIBBBBB", body)
        elif kind == b"IDAT":
            idat += body
    w, h, depth, ctype, _, _, interlace = head
    if depth != 8 or ctype not in (2, 6) or interlace:
        raise ValueError("unsupported PNG layout")
    bpp = 3 if ctype == 2 else 4
    raw, stride, rows, prev, p = zlib.decompress(idat), w * bpp, [], bytearray(w * bpp), 0
    for _ in range(h):
        f, line = raw[p], bytearray(raw[p + 1:p + 1 + stride])
        p += 1 + stride
        for i in range(stride):
            a = line[i - bpp] if i >= bpp else 0
            b = prev[i]
            c = prev[i - bpp] if i >= bpp else 0
            if f == 1:
                line[i] = (line[i] + a) & 255
            elif f == 2:
                line[i] = (line[i] + b) & 255
            elif f == 3:
                line[i] = (line[i] + ((a + b) >> 1)) & 255
            elif f == 4:
                pa, pb, pc = abs(b - c), abs(a - c), abs(a + b - 2 * c)
                line[i] = (line[i] + (a if pa <= pb and pa <= pc else b if pb <= pc else c)) & 255
        rows.append([(line[x * bpp], line[x * bpp + 1], line[x * bpp + 2]) for x in range(w)])
        prev = line
    return w, h, rows


def colours_from(png):
    """Average colour, and five colours from the upper edge to the lower one, of a picture."""
    small = png + ".small.png"
    subprocess.run(["sips", "-z", "30", "48", png, "--out", small], capture_output=True, check=True)
    w, h, rows = read_png(small)
    os.remove(small)

    def mean(pixels):
        n = max(1, len(pixels))
        return "#%02x%02x%02x" % tuple(int(round(sum(p[k] for p in pixels) / n)) for k in range(3))
    bands = [mean([p for row in rows[h * k // 5:h * (k + 1) // 5] for p in row]) for k in range(5)]
    return mean([p for row in rows for p in row]), bands


# --------------------------------------------------------------------------------------
# Commands
# --------------------------------------------------------------------------------------

def cmd_build(args):
    slugs = args.slug or all_slugs()
    if not slugs:
        raise Problem("there are no scene sources in %s yet" % HERE)
    for slug in slugs:
        build_one(slug)


def cmd_wrap(args):
    check_slug(args.slug)
    if not os.path.isfile(args.shader):
        raise Problem("cannot find the shader file %s" % args.shader)
    author, licence = args.author.strip(), args.license.strip()
    if not author or not licence:
        raise Problem("--author and --license must say who made the shader and under which licence. "
                      "Only wrap work whose licence allows commercial redistribution.")
    src = read(args.shader)
    prepare_web_shader(src, os.path.basename(args.shader))        # refuse early, before anything is written
    folder = source_dir(args.slug)
    if os.path.exists(folder) and not args.replace:
        raise Problem("there is already a scene source called %r; pass --replace to overwrite its shader and settings" % args.slug)
    colors = None
    if args.colors:
        colors = [c.strip() for c in args.colors.split(",") if c.strip()]
        if len(colors) < 2 or not all(HEX.match(c) for c in colors):
            raise Problem("--colors takes two or more #rrggbb colours, upper to lower, separated by commas")
    cfg = {
        "kind": "shader", "style": "web", "title": args.title.strip(),
        "tags": [t.strip().lower() for t in (args.tags or "").split(",") if t.strip()],
        "color": None, "author": author, "license": licence, "sourceUrl": args.source or None,
        "loop": args.loop, "fade": args.fade, "maxSide": args.max_side, "colors": colors, "poster": 20,
    }
    if os.path.abspath(args.shader) != os.path.abspath(os.path.join(folder, "shader.glsl")):
        write(os.path.join(folder, "shader.glsl"), src)
    save(args.slug, cfg)
    path = build_one(args.slug)
    binary = None if args.no_check else find_chrome(args.chrome)
    if binary:
        if not report_probe(args.slug, cfg, binary):
            raise Problem("the scene was written but its shader does not run. Correct %s and run "
                          "`python3 scenekit.py build %s`." % (os.path.join(folder, "shader.glsl"), args.slug))
    elif not args.no_check:
        say("    no Chrome found, so the shader was not test-compiled; open the scene in a browser to see it")
    say("Next: open %s to look at it, then: python3 scenekit.py poster %s" % (path, args.slug))
    if not colors:
        say("      (the fallback colours are neutral for now; `poster` takes them from the picture)")


def cmd_new_dom(args):
    check_slug(args.slug)
    folder = source_dir(args.slug)
    if os.path.exists(folder):
        raise Problem("there is already a scene source called %r" % args.slug)
    title = args.title.strip()
    page = fill(runtime("dom.html"), {"TITLE": htmllib.escape(title).replace("--", "- -"), "SLUG": args.slug, "STANDARD": STANDARD})
    cfg = {
        "kind": "dom", "title": title,
        "tags": [t.strip().lower() for t in (args.tags or "").split(",") if t.strip()],
        "color": None, "author": (args.author or "").strip(), "license": (args.license or "").strip(),
        "sourceUrl": args.source or None, "poster": 36540,
    }
    write(os.path.join(folder, "scene.html"), page)
    save(args.slug, cfg)
    path = build_one(args.slug)
    say("Next: edit %s (the places marked FILL IN), run `python3 scenekit.py build %s`,\n      and open %s to look."
        % (os.path.join(folder, "scene.html"), args.slug, path))


def cmd_poster(args):
    check_slug(args.slug)
    cfg = load(args.slug)
    binary = find_chrome(args.chrome)
    if not binary:
        raise Problem("no Chrome was found, so the poster cannot be captured. Install Google Chrome, or point at a "
                      "Chromium-based browser with --chrome /path/to/binary (or the CHROME environment variable).")
    if not shutil.which("sips"):
        raise Problem("`sips` (part of macOS) is needed to write the JPEG and was not found")
    scene = build_one(args.slug, quiet=True)
    t = args.t if args.t is not None else cfg.get("poster", 36540 if cfg["kind"] == "dom" else 20)
    if cfg["kind"] == "shader" and not report_probe(args.slug, cfg, binary):
        raise Problem("the poster was not captured, because it would show the fallback rather than the scene")
    with tempfile.TemporaryDirectory(prefix="scenekit-poster-") as tmp:
        shot = os.path.join(tmp, "shot.png")
        done = chrome(binary, scene, "t=%g" % t, ["--screenshot=" + shot])
        if not os.path.isfile(shot):
            raise Problem("Chrome did not produce a screenshot:\n" + done.stderr.decode("utf-8", "replace")[-600:])
        w, h, _ = struct.unpack(">II", open(shot, "rb").read()[16:24]) + (0,)
        if (w, h) != (POSTER_W, POSTER_H):
            subprocess.run(["sips", "-z", str(POSTER_H), str(POSTER_W), shot], capture_output=True, check=True)
        poster = os.path.join(OUT, args.slug, "poster.jpg")
        subprocess.run(["sips", "-s", "format", "jpeg", "-s", "formatOptions", "88", shot, "--out", poster],
                       capture_output=True, check=True)
        changed = False
        try:
            if not cfg.get("color") or (cfg["kind"] == "shader" and not cfg.get("colors") and not cfg.get("fallback")):
                average, bands = colours_from(shot)
                if not cfg.get("color"):
                    cfg["color"], changed = average, True
                if cfg["kind"] == "shader" and not cfg.get("colors") and not cfg.get("fallback"):
                    cfg["colors"], changed = bands, True
        except (ValueError, subprocess.CalledProcessError, OSError):
            say("    could not read colours from the picture; set \"color\" in scene.json by hand")
    if args.t is not None and cfg.get("poster") != args.t:
        cfg["poster"], changed = args.t, True
    if changed:
        save(args.slug, cfg)
        build_one(args.slug, quiet=True)
        say("    scene.json updated (poster moment and colours taken from the picture) and the scene rebuilt")
    say("%-14s poster at t=%g  %d bytes  %s" % (args.slug, t, os.path.getsize(poster), poster))


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="scenekit.py", formatter_class=argparse.RawDescriptionHelpFormatter,
        description=__doc__)
    sub = parser.add_subparsers(dest="command", metavar="command")

    p = sub.add_parser("build", help="rebuild finished scenes from their sources (all, or the ones named)",
                       description="Rebuild scenes/<slug>/scene.html and meta.json from scene-sources/<slug>/. "
                                   "With no slug, every scene is rebuilt. Posters are left alone.")
    p.add_argument("slug", nargs="*", help="scene to rebuild; none means all")
    p.set_defaults(run=cmd_build)

    p = sub.add_parser("wrap-shader", help="turn a web-style fragment shader into a scene that spans displays",
                       formatter_class=argparse.RawDescriptionHelpFormatter,
                       description="""Wrap a fragment shader written in the common web style:

    void mainImage(out vec4 fragColor, in vec2 fragCoord) { ... }

using iTime and iResolution (iMouse is always zero; iFrame, iTimeDelta, iFrameRate and
iDate are supplied too). The result is a canvas scene: the shader draws one picture the
size of the whole canvas, and each display shows its own part of it.

Shaders that need textures, buffers, sound or input (iChannel..., texture(...)) are
refused: a scene can load nothing. Comments are removed from the shader, because they
often hold web addresses, which a scene may not contain; credit goes into --author,
--license and --source instead.

A shader that does not repeat by itself is blended across the end of each loop, so the
restart is a slow dissolve rather than a jump. If it does repeat, give its period as
--loop and the join is invisible.""")
    p.add_argument("shader", help="the .glsl file")
    p.add_argument("--slug", required=True, help="folder name of the scene: lowercase, hyphens (soft-glow)")
    p.add_argument("--title", required=True, help="display title, two words at most (\"Soft Glow\")")
    p.add_argument("--author", required=True, help="who made the shader (required; there is no default)")
    p.add_argument("--license", required=True, help="the licence that allows commercial redistribution (required; no default)")
    p.add_argument("--source", metavar="URL", help="where the original is published")
    p.add_argument("--tags", metavar="a,b", help="two or three lowercase tags, separated by commas")
    p.add_argument("--loop", type=float, default=900, metavar="SECONDS", help="how long the shader's time runs before starting again (default 900)")
    p.add_argument("--fade", type=float, default=20, metavar="SECONDS", help="length of the dissolve across the end of the loop (default 20; 0 if the shader repeats exactly every --loop seconds)")
    p.add_argument("--max-side", type=int, default=MAX_SIDE, metavar="N", help="longest side of the render buffer in pixels (default and limit %d; lower is cheaper and softer)" % MAX_SIDE)
    p.add_argument("--colors", metavar="#a,#b", help="two or more #rrggbb colours, upper to lower, for the no-WebGL fallback; left out, `poster` takes them from the picture")
    p.add_argument("--replace", action="store_true", help="overwrite an existing scene source with the same slug")
    p.add_argument("--no-check", action="store_true", help="do not test-compile the shader in a local Chrome")
    p.add_argument("--chrome", metavar="PATH", help="the Chrome (or Chromium-based) binary to use")
    p.set_defaults(run=cmd_wrap)

    p = sub.add_parser("new-dom", help="start a per-display scene made of ordinary HTML, CSS and script",
                       description="Write scene-sources/<slug>/scene.html: a skeleton for a per-display scene (a clock, a "
                                   "calendar - anything that must appear whole on every display) with clearly marked places "
                                   "to fill in. The shared runtime supplies pause, the hidden-page stop, a once-a-minute "
                                   "update, an optional slow frame loop and a smooth background. It is built at once, so "
                                   "there is something to open; edit the source and run `build <slug>` to see changes.")
    p.add_argument("--slug", required=True, help="folder name of the scene: lowercase, hyphens (desk-calendar)")
    p.add_argument("--title", required=True, help="display title, two words at most")
    p.add_argument("--author", help="who made it; needed before the scene can be published")
    p.add_argument("--license", help="its licence; needed before the scene can be published")
    p.add_argument("--source", metavar="URL", help="where the original is published, if it is adapted from something")
    p.add_argument("--tags", metavar="a,b", help="two or three lowercase tags, separated by commas")
    p.set_defaults(run=cmd_new_dom)

    p = sub.add_parser("poster", help="capture poster.jpg (1920x1200) with a local Chrome",
                       description="Rebuild the scene, open it in a headless local Chrome frozen at one moment, and save "
                                   "scenes/<slug>/poster.jpg. For a shader scene --t is seconds into its loop; for a "
                                   "per-display scene it is seconds since local midnight (36540 is 10:09). The moment is "
                                   "remembered in scene.json, and missing colours are taken from the picture.")
    p.add_argument("slug")
    p.add_argument("--t", type=float, metavar="SECONDS", help="the moment to freeze (default: the one remembered in scene.json)")
    p.add_argument("--chrome", metavar="PATH", help="the Chrome (or Chromium-based) binary to use")
    p.set_defaults(run=cmd_poster)

    args = parser.parse_args(argv)
    if not getattr(args, "run", None):
        parser.print_help()
        return 2
    try:
        args.run(args)
    except Problem as e:
        print("scenekit: " + str(e), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
