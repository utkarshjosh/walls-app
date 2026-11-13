// The download buttons.
//
// Without script every button asks the server for "the build that suits most
// Macs" and that works. With script the page does better: it asks the catalog
// service which builds exist (GET /v1/releases/latest), works out what this
// computer is, points the buttons at the right file and says which one it is.
// Each link goes through /v1/download/<platform>, which counts the download
// and then sends the browser to the file.

type Build = { platform: string; label: string; filename: string; bytes: number; sha256: string; url: string };
type Release = { version: string; channel: "beta" | "stable"; notes: string; publishedAt: string; builds: Build[] };

type Os = "mac" | "windows" | "linux" | "phone" | "other";
type Chip = "arm64" | "x64" | "unknown";

const OS_NAMES: Record<Os, string> = { mac: "Mac", windows: "Windows", linux: "Linux", phone: "", other: "" };
const CHIP_NAMES: Record<string, string> = { arm64: "Apple silicon", x64: "Intel", universal: "Apple silicon and Intel" };

function detectOs(): Os {
  const ua = navigator.userAgent;
  if (/Android|iPhone|iPod|iPad/i.test(ua)) return "phone";
  // An iPad says it is a Mac. A Mac has no touch screen.
  if (/Macintosh|Mac OS X/i.test(ua)) return navigator.maxTouchPoints > 1 ? "phone" : "mac";
  if (/Windows/i.test(ua)) return "windows";
  if (/Linux|X11|CrOS/i.test(ua)) return "linux";
  return "other";
}

/**
 * Whether this computer has an ARM or an Intel-type chip. Browsers on the Mac
 * all claim to be Intel in their name, for old sites' sake, so this asks two
 * other places. Safari answers neither, and the result is then "unknown".
 */
async function detectChip(): Promise<Chip> {
  type Hints = { getHighEntropyValues?: (hints: string[]) => Promise<{ architecture?: string }> };
  const hints = (navigator as Navigator & { userAgentData?: Hints }).userAgentData;
  try {
    const arch = (await hints?.getHighEntropyValues?.(["architecture"]))?.architecture;
    if (arch === "arm") return "arm64";
    if (arch === "x86") return "x64";
  } catch {
    // Not offered, or refused: try the graphics chip instead.
  }
  try {
    const gl = document.createElement("canvas").getContext("webgl");
    const info = gl?.getExtension("WEBGL_debug_renderer_info");
    const renderer = String((gl && info && gl.getParameter(info.UNMASKED_RENDERER_WEBGL)) ?? "");
    gl?.getExtension("WEBGL_lose_context")?.loseContext();
    if (/Apple M\d/i.test(renderer)) return "arm64";
    if (/Intel|AMD|Radeon|NVIDIA|GeForce/i.test(renderer)) return "x64";
  } catch {
    // No WebGL: leave it unknown.
  }
  return "unknown";
}

function size(bytes: number): string {
  return bytes >= 1e9 ? `${(bytes / 1e9).toFixed(1)} GB` : `${Math.max(1, Math.round(bytes / 1e6))} MB`;
}

const chipOf = (build: Build) => build.platform.split("-")[1] ?? "";
const versionLabel = (release: Release) => `${release.channel === "beta" ? "Beta" : "Version"} ${release.version}`;

type Choice = {
  /** The build the big button fetches; null when none suits this computer. */
  build: Build | null;
  /** Shown under the button when something needs saying. */
  note: string;
  /** Another build worth offering in that note, such as the Intel one. */
  other: Build | null;
};

/** Picks the build for this computer out of the ones that exist. */
export function choose(builds: Build[], os: Os, chip: Chip): Choice {
  const of = (platform: string) => builds.find((b) => b.platform === platform) ?? null;
  const mac = { universal: of("mac-universal"), arm64: of("mac-arm64"), x64: of("mac-x64") };
  const anyMac = mac.universal ?? mac.arm64 ?? mac.x64;

  if (os === "phone") {
    return { build: null, other: null, note: "Still runs on a Mac. Open this page on your Mac to download it." };
  }
  if (os === "windows" || os === "linux") {
    const own = of(`${os}-${chip === "arm64" ? "arm64" : "x64"}`) ?? of(`${os}-x64`) ?? of(`${os}-arm64`);
    if (own) return { build: own, other: null, note: "" };
    // Nothing for this system. The Mac download stays in plain sight all the
    // same: people fetch it here for a Mac they have elsewhere, and a page
    // with no button reads as "there is nothing to download".
    return {
      build: anyMac,
      other: null,
      note: `Still is a Mac app for now. There is no ${OS_NAMES[os]} version yet${anyMac ? ", so this is the Mac download" : ""}.`,
    };
  }
  if (!anyMac) return { build: null, other: null, note: "There is no Mac build of this release yet." };
  // A system this page does not recognise, or one file for every Mac:
  // nothing to work out.
  if (os !== "mac" || mac.universal) return { build: anyMac, other: null, note: "" };

  const siliconOnly = "This beta needs a Mac with Apple silicon (M1 or later). There is no Intel build yet.";
  if (chip === "unknown") {
    // Most Macs sold since late 2020 have Apple silicon, so offer that build
    // and say how to get the other. An Intel build runs on either.
    if (!mac.arm64) return { build: mac.x64, other: null, note: "" };
    return { build: mac.arm64, other: mac.x64, note: mac.x64 ? "On a Mac with an Intel chip?" : siliconOnly };
  }
  if (mac[chip]) return { build: mac[chip], other: null, note: "" };
  // The only build is for the other kind of chip.
  return chip === "x64"
    ? { build: null, other: null, note: siliconOnly }
    : { build: mac.x64, other: null, note: "This is the Intel build. It runs on your Mac through Rosetta." };
}

/** Wires up every download button on the page and the list of all builds. */
export async function setUpDownloads(): Promise<void> {
  const buttons = [...document.querySelectorAll<HTMLAnchorElement>("a[data-download]")];
  const meta = document.querySelector<HTMLElement>("#get-meta")!;
  const note = document.querySelector<HTMLElement>("#get-note")!;
  const list = document.querySelector<HTMLElement>("#builds")!;

  const say = (text: string, link?: { text: string; href: string }) => {
    note.replaceChildren(text);
    if (link) {
      const a = document.createElement("a");
      a.className = "quiet";
      a.href = link.href;
      a.textContent = link.text;
      note.append(" ", a);
    }
    note.hidden = !text;
  };
  // A button with nothing to fetch: the one in the closing section goes, and
  // the others lead to that section, where the reason is given.
  const noDownload = (label = "See the downloads") => {
    for (const b of buttons) {
      if (b.closest(".get")) b.hidden = true;
      else {
        b.href = "#get";
        b.textContent = label;
      }
    }
  };

  let release: Release | null = null;
  try {
    const response = await fetch("/v1/releases/latest", { headers: { accept: "application/json" } });
    if (response.ok) release = (await response.json()) as Release;
    else if (response.status !== 404) throw new Error(String(response.status));
  } catch {
    // The service cannot be reached. The plain links still work if it comes back.
    return;
  }
  if (!release || release.builds.length === 0) {
    noDownload("Coming soon");
    meta.textContent = "";
    say("The first build is being prepared. Check back soon.");
    list.replaceChildren(Object.assign(document.createElement("p"), { textContent: "Nothing to download yet." }));
    return;
  }

  // Every build of this release, for people who want a particular one.
  const rows = release.builds.map((b) => {
    const row = document.createElement("div");
    row.className = "build";
    const name = document.createElement("div");
    const link = Object.assign(document.createElement("a"), { className: "quiet", href: b.url, textContent: b.label });
    const file = Object.assign(document.createElement("small"), { textContent: `${b.filename} · ${size(b.bytes)}` });
    name.append(link, file);
    // For checking the file after it has arrived: `shasum -a 256 <file>`.
    const sum = Object.assign(document.createElement("code"), { textContent: b.sha256, title: "SHA-256 of the file" });
    row.append(name, sum);
    return row;
  });
  const about = document.createElement("p");
  about.className = "builds__version";
  const date = new Date(release.publishedAt).toLocaleDateString("en", { day: "numeric", month: "long", year: "numeric" });
  about.textContent = `${versionLabel(release)}, published ${date}.` + (release.notes ? ` ${release.notes}` : "");
  list.replaceChildren(about, ...rows);

  const pick = choose(release.builds, detectOs(), await detectChip());
  if (pick.build) {
    const system = pick.build.platform.split("-")[0];
    for (const b of buttons) {
      b.href = pick.build.url;
      b.textContent = `Download for ${OS_NAMES[system as Os] || "Mac"}`;
    }
    const chip = system === "mac" ? CHIP_NAMES[chipOf(pick.build)] : pick.build.label;
    meta.textContent = [versionLabel(release), chip, size(pick.build.bytes)].filter(Boolean).join(" · ");
  } else {
    noDownload();
    meta.textContent = versionLabel(release);
  }
  say(pick.note, pick.other ? { text: `Download the ${CHIP_NAMES[chipOf(pick.other)] ?? ""} build for Mac`.replace("  ", " "), href: pick.other.url } : undefined);
}
