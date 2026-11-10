#!/usr/bin/env python3
"""Assemble each scene.html from the shared harness and its fragment shader."""
import json, os, sys

HERE = os.path.dirname(os.path.abspath(__file__))
# Scenes are written next to this folder, where `walls seed` picks them up.
OUT = os.path.normpath(os.path.join(HERE, "..", "scenes"))

SCENES = {
    "slow-aurora": dict(
        title="Slow Aurora", maxside=1600, loop=240, bg="#02030a", glsl="aurora.glsl",
        fallback="linear-gradient(180deg,#02030a 0%,#06141f 28%,#0f4a3c 47%,#07202a 62%,#040b14 100%)",
        tags=["night", "aurora", "cool"], color="#14463f"),
    "tidal": dict(
        title="Tidal", maxside=1100, loop=240, bg="#0b0d1a", glsl="tidal.glsl",
        fallback="linear-gradient(200deg,#0d0e1b 0%,#4c2640 35%,#c97878 65%,#f5dbb8 100%)",
        tags=["abstract", "gradient", "warm"], color="#a8676a"),
    "dune-wind": dict(
        title="Dune Wind", maxside=1600, loop=300, bg="#5a3324", glsl="dune.glsl",
        fallback="linear-gradient(180deg,#e9d6bf 0%,#efdcc4 38%,#d9ab80 55%,#b07850 78%,#6e3f2b 100%)",
        tags=["desert", "warm", "light"], color="#d2ae90"),
    "ink-wash": dict(
        title="Ink Wash", maxside=1300, loop=360, bg="#37383b", glsl="ink.glsl",
        fallback="linear-gradient(160deg,#ede7db 0%,#e6e0d4 55%,#b9b5ac 80%,#77756f 100%)",
        tags=["monochrome", "minimal", "light"], color="#c9c5bb"),
    "first-light": dict(
        title="First Light", maxside=1600, loop=240, bg="#080a18", glsl="dawn.glsl",
        fallback="linear-gradient(180deg,#1a2444 0%,#6b5878 34%,#e5a385 56%,#57506f 62%,#1a1c30 100%)",
        tags=["dawn", "horizon", "calm"], color="#66597a"),
}

def read(name):
    with open(os.path.join(HERE, name)) as f:
        return f.read()

def main(only=None):
    harness, head, tail = read("harness.html"), read("common_head.glsl"), read("common_main.glsl")
    for slug, s in SCENES.items():
        if only and slug not in only:
            continue
        path = os.path.join(HERE, s["glsl"])
        if not os.path.exists(path):
            print("skip", slug)
            continue
        frag = head + "\n" + read(s["glsl"]) + tail
        html = (harness.replace("{{TITLE}}", s["title"]).replace("{{LOOP}}", str(s["loop"])).replace("{{MAXSIDE}}", str(s["maxside"]))
                .replace("{{BG}}", s["bg"]).replace("{{FALLBACK}}", s["fallback"])
                .replace("{{FRAG}}", frag.strip()))
        d = os.path.join(OUT, slug)
        os.makedirs(d, exist_ok=True)
        with open(os.path.join(d, "scene.html"), "w") as f:
            f.write(html)
        with open(os.path.join(d, "meta.json"), "w") as f:
            json.dump({"title": s["title"], "tags": s["tags"], "color": s["color"]}, f)
            f.write("\n")
        print(slug, len(html.encode()), "bytes")

if __name__ == "__main__":
    main(sys.argv[1:])
