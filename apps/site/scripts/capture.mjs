// Takes the pictures the page needs of the product itself, with headless Chrome:
//
//   public/media/app/library.jpg, discover.jpg, rotation.jpg   the app's three tabs
//   public/media/hero.jpg    the 3D hero on its own, shown until the 3D has loaded
//   public/media/og.jpg      the hero with its headline, for link previews
//
// Run it after the interface or the catalog changes:
//
//   npm run capture
//
// It needs three things already running (start them from the repository root):
//   the catalog      go run -C apps/api ./cmd/walls serve      (port 8787)
//   the app preview  npm --prefix apps/web run dev             (port 5173)
//   this page        npm --prefix apps/site run dev            (port 5180)
//
// The app pictures come from the browser preview of the interface, which is the
// same interface the Mac app shows. The preview-only controls are hidden first.
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, "..", "public", "media");
const APP = process.env.STILL_APP_URL ?? "http://localhost:5173/";
const SITE = process.env.STILL_SITE_URL ?? "http://localhost:5180/";
const CHROME =
  process.env.CHROME ?? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
/** Collections saved from Discover before the Library picture is taken. */
const SAVE_PACKS = Number(process.env.STILL_SAVE_PACKS ?? 4);

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function reachable(url) {
  try {
    await fetch(url);
    return true;
  } catch {
    return false;
  }
}

/** One Chrome, driven over its debugging port. */
async function browser() {
  if (!existsSync(CHROME)) throw new Error(`Chrome was not found at ${CHROME}. Set CHROME to a Chromium-based browser.`);
  const port = 9300 + Math.floor(Math.random() * 500);
  const profile = mkdtempSync(join(tmpdir(), "still-capture-"));
  const proc = spawn(
    CHROME,
    ["--headless=new", `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, "--hide-scrollbars", "--no-first-run",
      "--no-default-browser-check", "--enable-gpu", "--use-angle=metal", "--ignore-gpu-blocklist", "about:blank"],
    { stdio: "ignore" },
  );
  let info;
  for (let i = 0; i < 80 && !info; i++) {
    await sleep(150);
    try {
      info = await (await fetch(`http://127.0.0.1:${port}/json/version`)).json();
    } catch {}
  }
  if (!info) throw new Error("Chrome did not start.");
  const ws = new WebSocket(info.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = reject;
  });
  let id = 0;
  const waiting = new Map();
  const problems = [];
  ws.onmessage = (m) => {
    const msg = JSON.parse(m.data);
    if (msg.id && waiting.has(msg.id)) {
      const { resolve, reject } = waiting.get(msg.id);
      waiting.delete(msg.id);
      msg.error ? reject(new Error(JSON.stringify(msg.error))) : resolve(msg.result);
    } else if (msg.method === "Runtime.exceptionThrown") {
      problems.push(msg.params.exceptionDetails.exception?.description ?? msg.params.exceptionDetails.text);
    } else if (msg.method === "Runtime.consoleAPICalled" && msg.params.type === "error") {
      problems.push(msg.params.args.map((a) => a.value ?? a.description ?? "").join(" ").slice(0, 600));
    }
  };
  const send = (method, params = {}, sessionId) =>
    new Promise((resolve, reject) => {
      const n = ++id;
      waiting.set(n, { resolve, reject });
      ws.send(JSON.stringify({ id: n, method, params, sessionId }));
    });

  async function page(url, width, height, scale) {
    const { targetId } = await send("Target.createTarget", { url: "about:blank" });
    const { sessionId } = await send("Target.attachToTarget", { targetId, flatten: true });
    const call = (method, params) => send(method, params, sessionId);
    await call("Emulation.setDeviceMetricsOverride", { width, height, deviceScaleFactor: scale, mobile: false });
    await call("Page.enable");
    await call("Runtime.enable");
    await call("Page.navigate", { url });
    await sleep(2500);
    return {
      async run(expression) {
        const res = await call("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
        if (res.exceptionDetails) throw new Error(res.exceptionDetails.exception?.description ?? res.exceptionDetails.text);
        return res.result?.value;
      },
      async shot(file, quality = 86) {
        const { data } = await call("Page.captureScreenshot", { format: "jpeg", quality });
        writeFileSync(file, Buffer.from(data, "base64"));
        console.log("  wrote", file.replace(join(here, "..") + "/", ""));
      },
      close: () => send("Target.closeTarget", { targetId }),
    };
  }

  return {
    page,
    problems,
    async close() {
      ws.close();
      proc.kill();
      await sleep(300);
      rmSync(profile, { recursive: true, force: true });
    },
  };
}

// Helpers that run inside the app preview. Its tabs and buttons act on pointer
// events, so a plain click() is not enough.
const IN_APP = `
window.__press = (label, exact = true) => {
  const all = [...document.querySelectorAll('button,[role=tab],[role=radio],a')];
  const el = all.find((b) => exact ? b.textContent.trim() === label : b.textContent.trim().startsWith(label));
  if (!el) return false;
  const r = el.getBoundingClientRect();
  const o = { bubbles: true, cancelable: true, button: 0, clientX: r.x + r.width / 2, clientY: r.y + r.height / 2, pointerId: 1, pointerType: 'mouse', isPrimary: true };
  el.dispatchEvent(new PointerEvent('pointerdown', o));
  el.dispatchEvent(new MouseEvent('mousedown', o));
  el.dispatchEvent(new PointerEvent('pointerup', o));
  el.dispatchEvent(new MouseEvent('mouseup', o));
  el.dispatchEvent(new MouseEvent('click', o));
  return true;
};
// Hide what only the browser preview has, and anything transient.
window.__tidy = () => {
  for (const el of document.querySelectorAll('button, p, div, span')) {
    const text = el.textContent.trim();
    if (text === 'Preview tools' || text.startsWith('This is the browser preview')) el.style.visibility = 'hidden';
  }
  for (const el of document.querySelectorAll('[role=status], [data-toast]')) el.style.display = 'none';
  document.activeElement?.blur?.();
};
true`;

async function captureApp(chrome) {
  console.log("The app");
  mkdirSync(join(out, "app"), { recursive: true });
  const page = await chrome.page(APP, 1280, 800, 2);
  // Two displays, so the pictures show what the page talks about.
  await page.run(`localStorage.setItem('still.preview.displays', '2'); location.reload(); true`);
  await sleep(4000);
  await page.run(IN_APP);

  await page.run(`__press('Discover')`);
  await sleep(4000);
  for (let i = 0; i < SAVE_PACKS; i++) {
    if (!(await page.run(`__press('Save all', false)`))) break;
    await sleep(1500);
  }
  await sleep(2500);
  await page.run(`window.scrollTo(0, 0); document.querySelector('main')?.scrollTo?.(0, 0); __tidy(); true`);
  await sleep(600);
  await page.shot(join(out, "app", "discover.jpg"));

  await page.run(`__press('Library')`);
  await sleep(2500);
  await page.run(`__tidy(); true`);
  await sleep(400);
  await page.shot(join(out, "app", "library.jpg"));

  await page.run(`__press('Rotation')`);
  await sleep(2000);
  await page.run(`__press('One across all')`);
  await sleep(2500);
  await page.run(`__tidy(); true`);
  await sleep(400);
  await page.shot(join(out, "app", "rotation.jpg"));
  await page.close();
}

async function captureHero(chrome) {
  console.log("The hero");
  // Link preview: the hero as visitors first see it, at the size link cards use.
  let page = await chrome.page(SITE, 1200, 630, 2);
  await sleep(4500);
  await page.shot(join(out, "og.jpg"), 84);
  await page.close();

  // Stand-in picture: the 3D alone, since the text is drawn over it by the page.
  page = await chrome.page(SITE, 1600, 1000, 1);
  await sleep(4500);
  await page.run(`for (const s of ['.nav', '.hero__copy', '.hero__steps', '.hero__fallback']) document.querySelector(s).style.visibility = 'hidden'; true`);
  await sleep(300);
  await page.shot(join(out, "hero.jpg"), 82);
  await page.close();
}

const only = process.argv[2]; // "app" or "hero" to take just those
const chrome = await browser();
try {
  if (only !== "hero") {
    if (await reachable(APP)) await captureApp(chrome);
    else console.log(`Skipped the app pictures: nothing is answering at ${APP}`);
  }
  if (only !== "app") {
    if (await reachable(SITE)) await captureHero(chrome);
    else console.log(`Skipped the hero pictures: nothing is answering at ${SITE}`);
  }
  if (chrome.problems.length) console.log("Pages reported problems:\n  " + [...new Set(chrome.problems)].join("\n  "));
} finally {
  await chrome.close();
}
