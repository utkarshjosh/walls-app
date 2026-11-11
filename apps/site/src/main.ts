import "./styles.css";
import media from "./media.json";
import { EACH_DISPLAY, HERO_SCENE, LIVE_SCENES, STILLS } from "./content";
import type { Hero } from "./hero";

const root = document.documentElement;
root.classList.add("js");

const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
const $ = <T extends Element>(selector: string, within: ParentNode = document) => within.querySelector<T>(selector)!;
const clamp = (x: number, lo = 0, hi = 1) => Math.min(hi, Math.max(lo, x));

// ---- hero -------------------------------------------------------------------

const heroEl = $<HTMLElement>(".hero");
const nav = $<HTMLElement>(".nav");
const beats = [...document.querySelectorAll<HTMLElement>(".beat")];
const steps = [...document.querySelectorAll<HTMLButtonElement>(".hero__steps button")];

// Where in the pinned hero each state sits, and where a step button scrolls to.
const BEAT_STARTS = [0, 0.24, 0.6];
const BEAT_STOPS = [0, 0.46, 0.9];

let hero: Hero | null = null;
let beat = -1;

function heroProgress(): number {
  const r = heroEl.getBoundingClientRect();
  const span = r.height - window.innerHeight;
  return span > 0 ? clamp(-r.top / span) : 0;
}

function showBeat(next: number) {
  if (next === beat) return;
  beat = next;
  beats.forEach((el, i) => el.classList.toggle("is-on", i === next));
  steps.forEach((el, i) => (i === next ? el.setAttribute("aria-current", "true") : el.removeAttribute("aria-current")));
}

let queued = false;
function onScroll() {
  if (queued) return;
  queued = true;
  requestAnimationFrame(() => {
    queued = false;
    const p = heroProgress();
    hero?.progress(p);
    showBeat(p >= BEAT_STARTS[2] ? 2 : p >= BEAT_STARTS[1] ? 1 : 0);
    nav.classList.toggle("is-solid", heroEl.getBoundingClientRect().bottom < 72);
  });
}
window.addEventListener("scroll", onScroll, { passive: true });
window.addEventListener("resize", onScroll, { passive: true });
onScroll();

steps.forEach((button, i) =>
  button.addEventListener("click", () => {
    const span = heroEl.offsetHeight - window.innerHeight;
    window.scrollTo({ top: heroEl.offsetTop + span * BEAT_STOPS[i], behavior: reducedMotion ? "auto" : "smooth" });
  }),
);

// three.js arrives after the text is on screen. If it cannot start, the still
// picture underneath stays and the page works without it.
const ready = () => root.classList.add("ready");

// The hero runs a real scene from the catalog, straight from its source.
type SceneSettings = { loop?: number; poster?: number; color?: string };
const shaders = import.meta.glob<string>("../../api/seed/scene-sources/*/shader.glsl", { query: "?raw", import: "default" });
const settings = import.meta.glob<SceneSettings>("../../api/seed/scene-sources/*/scene.json", { import: "default" });
const sourceOf = (slug: string, file: string) => `../../api/seed/scene-sources/${slug}/${file}`;

function glowOf(hex: string | undefined): [number, number, number] {
  const n = parseInt((hex ?? "#806060").slice(1), 16);
  const rgb = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((v) => v / 255);
  // Light in a room is brighter and less saturated than the picture it comes from.
  const peak = Math.max(...rgb, 0.001);
  return rgb.map((v) => 0.35 + 0.5 * (v / peak)) as [number, number, number];
}

Promise.all([import("./hero"), shaders[sourceOf(HERO_SCENE, "shader.glsl")](), settings[sourceOf(HERO_SCENE, "scene.json")]()])
  .then(([{ createHero }, source, cfg]) => {
    hero = createHero({
      canvas: $<HTMLCanvasElement>(".hero__canvas"),
      stage: $<HTMLElement>(".hero__stage"),
      scene: { source, loop: cfg.loop ?? 900, still: cfg.poster ?? 20, glow: glowOf(cfg.color) },
      each: EACH_DISPLAY.map((slug) => `/media/stills/${slug}.jpg?v=${media.stills.find((s) => s.slug === slug)?.v ?? ""}`) as [string, string],
      reducedMotion,
    });
    hero.progress(heroProgress());
    root.classList.add("gl");

    let inView = true;
    const sync = () => hero?.running(inView && !document.hidden);
    new IntersectionObserver(([entry]) => {
      inView = entry.isIntersecting;
      sync();
    }).observe(heroEl);
    document.addEventListener("visibilitychange", sync);

    requestAnimationFrame(() => {
      hero?.reveal();
      ready();
    });
  })
  .catch((error) => {
    console.warn("The 3D scene could not start; showing the still picture instead.", error);
    ready();
  });
// Never leave the headline hidden if the scene is slow to load.
window.setTimeout(ready, 2500);

// ---- live scenes ------------------------------------------------------------

type Scene = (typeof media.scenes)[number];
const scenes = LIVE_SCENES.map((slug) => media.scenes.find((s) => s.slug === slug)).filter((s): s is Scene => !!s);

const liveFrame = $<HTMLElement>(".live__frame");
const liveScene = $<HTMLIFrameElement>(".live__scene");
const liveTitle = $<HTMLElement>(".live__title");
const livePause = $<HTMLButtonElement>(".live__pause");
const livePicker = $<HTMLElement>(".live__picker");

const live = { slug: "", v: "", visible: false, paused: reducedMotion, loaded: "" };

// A scene takes its state from the part of its address after "#". Changing only
// that part does not reload it, so pausing is instant.
function applyLive() {
  if (!live.slug) return;
  const stopped = live.paused || !live.visible;
  if (!live.visible && live.loaded !== live.slug) return; // not on screen yet: do not load it
  if (live.loaded !== live.slug) {
    // The frame shows the scene's own flat colour until the scene has drawn,
    // then the scene fades in. A poster is not used as a stand-in: it shows a
    // different moment, so swapping it for the running scene looks like a jump.
    const slug = live.slug;
    liveScene.classList.remove("is-on");
    liveScene.addEventListener(
      "load",
      () => window.setTimeout(() => live.slug === slug && liveScene.classList.add("is-on"), 220),
      { once: true },
    );
    live.loaded = live.slug;
  }
  liveScene.src = `/media/scenes/${live.slug}.html?v=${live.v}#paused=${stopped ? 1 : 0}`;
  livePause.textContent = live.paused ? "Play" : "Pause";
  livePause.setAttribute("aria-pressed", String(live.paused));
}

function chooseLive(scene: Scene) {
  live.slug = scene.slug;
  live.v = scene.v;
  liveFrame.style.backgroundColor = scene.color ?? "";
  liveTitle.textContent = scene.title;
  for (const b of livePicker.querySelectorAll("button")) b.setAttribute("aria-pressed", String(b.dataset.slug === scene.slug));
  applyLive();
}

for (const scene of scenes) {
  const button = document.createElement("button");
  button.type = "button";
  button.dataset.slug = scene.slug;
  const img = document.createElement("img");
  img.src = `/media/posters/${scene.slug}.jpg?v=${scene.v}`;
  img.alt = "";
  img.loading = "lazy";
  const label = document.createElement("span");
  label.textContent = scene.title;
  button.append(img, label);
  button.addEventListener("click", () => chooseLive(scene));
  livePicker.append(button);
}

livePause.addEventListener("click", () => {
  live.paused = !live.paused;
  applyLive();
});
new IntersectionObserver(
  ([entry]) => {
    live.visible = entry.isIntersecting;
    applyLive();
  },
  { rootMargin: "200px 0px" },
).observe(liveFrame);
if (scenes[0]) chooseLive(scenes[0]);

// ---- wallpapers -------------------------------------------------------------

const strip = $<HTMLElement>(".walls__strip");
type Tile = { src: string; title: string; note: string; ratio: number };
type Still = { slug: string; title: string; color: string | null; width: number; height: number; v: string };
const allStills = media.stills as Still[];
const chosen = STILLS.length ? STILLS.map((slug) => allStills.find((s) => s.slug === slug)).filter((s): s is Still => !!s) : allStills;
const tiles: Tile[] = chosen.length
  ? chosen.map((s) => ({ src: `/media/stills/${s.slug}.jpg?v=${s.v}`, title: s.title, note: `${s.width} × ${s.height}`, ratio: s.width / s.height }))
  : media.scenes
      .filter((s) => !s.tags.includes("clock"))
      .map((s) => ({ src: `/media/posters/${s.slug}.jpg?v=${s.v}`, title: s.title, note: "Live", ratio: 1.6 }));

for (const tile of tiles) {
  const figure = document.createElement("figure");
  figure.className = "wall";
  const img = document.createElement("img");
  img.src = tile.src;
  img.alt = `${tile.title}, a wallpaper from the Still catalog`;
  img.loading = "lazy";
  img.style.aspectRatio = String(tile.ratio);
  const caption = document.createElement("figcaption");
  const name = document.createElement("span");
  name.textContent = tile.title;
  const note = document.createElement("small");
  note.textContent = tile.note;
  caption.append(name, note);
  figure.append(img, caption);
  strip.append(figure);
}

// ---- the app ----------------------------------------------------------------

const SHOTS: Record<string, string> = {
  library: "The Library tab of Still, showing collections of wallpapers",
  discover: "The Discover tab of Still, showing the catalog of wallpapers and live scenes",
  rotation: "The Rotation tab of Still, showing two displays and the schedule",
};
const tabs = [...document.querySelectorAll<HTMLButtonElement>(".app__tabs button")];
const shot = $<HTMLImageElement>(".app__window img");
const panel = $<HTMLElement>(".app__window");

function selectTab(tab: HTMLButtonElement, focus = false) {
  const name = tab.dataset.shot!;
  for (const t of tabs) {
    const on = t === tab;
    t.setAttribute("aria-selected", String(on));
    t.tabIndex = on ? 0 : -1;
  }
  panel.setAttribute("aria-labelledby", tab.id);
  if (focus) tab.focus();
  shot.classList.add("is-swapping");
  const next = new Image();
  next.onload = next.onerror = () => {
    shot.src = next.src;
    shot.alt = SHOTS[name];
    shot.classList.remove("is-swapping");
  };
  next.src = `/media/app/${name}.jpg`;
}
tabs.forEach((tab, i) => {
  tab.addEventListener("click", () => selectTab(tab));
  tab.addEventListener("keydown", (e) => {
    const step = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    selectTab(tabs[(i + step + tabs.length) % tabs.length], true);
  });
});

// ---- the early build --------------------------------------------------------

const joinForm = $<HTMLFormElement>(".join__form");
const joinEmail = $<HTMLInputElement>("#join-email");
const joinTrap = $<HTMLInputElement>("#join-website");
const joinButton = $<HTMLButtonElement>("button", joinForm);
const joinNote = $<HTMLElement>(".join__note");

function say(text: string, state: "" | "is-error" | "is-done" = "") {
  joinNote.textContent = text;
  joinNote.className = `join__note ${state}`.trim();
}

joinForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (joinButton.disabled) return;
  const email = joinEmail.value.trim();
  if (!email || !joinEmail.checkValidity()) {
    say("That does not look like an email address.", "is-error");
    joinEmail.focus();
    return;
  }
  joinButton.disabled = true;
  say("Sending…");
  try {
    const response = await fetch("/v1/signups", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ email, website: joinTrap.value }),
    });
    if (response.ok) {
      joinEmail.value = "";
      say("You are on the list. We will write when the build is ready.", "is-done");
      return; // the button stays off: one address per visit is enough
    }
    const body = (await response.json().catch(() => null)) as { message?: string } | null;
    say(body?.message ?? "That did not go through. Try again in a moment.", "is-error");
  } catch {
    say("That did not go through. Check your connection and try again.", "is-error");
  }
  joinButton.disabled = false;
});

// ---- one tick per visit -----------------------------------------------------

// Tells the server the page was opened, so the owner can see how many people
// came by. It carries nothing: no cookie, no identifier, no address of the page
// they came from. Once per tab, so reloading does not count twice.
try {
  if (!sessionStorage.getItem("still:seen")) {
    sessionStorage.setItem("still:seen", "1");
    navigator.sendBeacon("/v1/visits");
  }
} catch {
  // Storage is switched off in this browser: skip the count rather than guess.
}
