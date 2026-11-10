// Copies what the page shows from the catalog's seed folder into this site:
// the live scenes themselves (the page runs the real files), their posters, and
// web-sized copies of the still wallpapers. Run it after scenes or stills change:
//
//   npm run media
//
// Needs `sips` (part of macOS) to resize pictures. The output is committed, so
// the site builds on any machine without running this.
import { execFileSync } from "node:child_process";
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const seed = join(here, "..", "..", "api", "seed");
const out = join(here, "..", "public", "media");

function resize(from, to, longSide, quality) {
  execFileSync("sips", ["-Z", String(longSide), "-s", "format", "jpeg", "-s", "formatOptions", String(quality), from, "--out", to], { stdio: "ignore" });
}

function pixelSize(file) {
  const text = execFileSync("sips", ["-g", "pixelWidth", "-g", "pixelHeight", file], { encoding: "utf8" });
  return { width: Number(/pixelWidth: (\d+)/.exec(text)[1]), height: Number(/pixelHeight: (\d+)/.exec(text)[1]) };
}

for (const folder of ["scenes", "posters", "stills"]) {
  rmSync(join(out, folder), { recursive: true, force: true });
  mkdirSync(join(out, folder), { recursive: true });
}

const scenes = [];
const scenesDir = join(seed, "scenes");
for (const slug of readdirSync(scenesDir).sort()) {
  const dir = join(scenesDir, slug);
  if (!statSync(dir).isDirectory() || !existsSync(join(dir, "scene.html"))) continue;
  const meta = JSON.parse(readFileSync(join(dir, "meta.json"), "utf8"));
  cpSync(join(dir, "scene.html"), join(out, "scenes", `${slug}.html`));
  resize(join(dir, "poster.jpg"), join(out, "posters", `${slug}.jpg`), 1600, 80);
  scenes.push({ slug, title: meta.title, tags: meta.tags ?? [], color: meta.color, layout: meta.layout });
}

// Stills: the pictures listed in seed/stills/stills.json, in that order.
const stills = [];
const stillsDir = join(seed, "stills");
if (existsSync(join(stillsDir, "stills.json"))) {
  for (const still of JSON.parse(readFileSync(join(stillsDir, "stills.json"), "utf8")).stills) {
    const slug = still.file.replace(/\.jpg$/, "");
    const file = join(stillsDir, still.file);
    const { width, height } = pixelSize(file);
    resize(file, join(out, "stills", still.file), 1800, 80);
    stills.push({ slug, title: still.title, color: still.color ?? null, width, height });
  }
}

writeFileSync(join(here, "..", "src", "media.json"), JSON.stringify({ scenes, stills }, null, 2) + "\n");
console.log(`${scenes.length} scenes, ${stills.length} stills copied to public/media`);
