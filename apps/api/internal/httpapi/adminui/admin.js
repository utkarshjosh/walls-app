// The admin portal. Plain script, no build step. Everything shown comes from
// /v1/admin; text from the server is always placed with textContent, never as
// markup, because addresses and notes are typed by other people.
"use strict";

const $ = (id) => document.getElementById(id);
const el = (tag, props = {}, ...children) => {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
};

const errorBox = $("error");
function showError(message) {
  errorBox.textContent = message || "";
  errorBox.hidden = !message;
}

// One place talks to the server. A 401 anywhere means the sign-in ran out.
async function api(path, options = {}) {
  const init = { method: options.method || "GET", credentials: "same-origin", headers: { accept: "application/json" } };
  if (options.file) {
    // A build being uploaded: the file itself is the request.
    init.headers["content-type"] = "application/octet-stream";
    init.body = options.file;
  } else if (options.body !== undefined) {
    init.headers["content-type"] = "application/json";
    init.body = JSON.stringify(options.body);
  }
  let response;
  try {
    response = await fetch("/v1/admin" + path, init);
  } catch {
    throw new Error("The server cannot be reached.");
  }
  if (response.status === 401 && path !== "/login") {
    showGate(true);
    throw new Error("Sign in to continue.");
  }
  if (response.status === 204) return null;
  const data = await response.json().catch(() => null);
  if (!response.ok) throw new Error((data && data.message) || "The server returned an unexpected answer.");
  return data;
}

// Runs an action started by a click and shows what went wrong, if anything.
function act(fn) {
  return async (event) => {
    if (event) event.preventDefault();
    showError("");
    try {
      await fn(event);
    } catch (error) {
      showError(error.message);
    }
  };
}

const number = new Intl.NumberFormat("en");
const dayLabel = (iso) => new Date(iso + "T00:00:00Z").toLocaleDateString("en", { day: "numeric", month: "short", timeZone: "UTC" });
const whenLabel = (iso) =>
  new Date(iso).toLocaleString("en", { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });
function sizeLabel(bytes) {
  if (bytes >= 1e9) return (bytes / 1e9).toFixed(2) + " GB";
  if (bytes >= 1e6) return (bytes / 1e6).toFixed(1) + " MB";
  return Math.max(1, Math.round(bytes / 1e3)) + " kB";
}

// ---- signing in ---------------------------------------------------------------

function showGate(canSignIn) {
  $("app").hidden = true;
  $("gate").hidden = false;
  $("login").hidden = !canSignIn;
  $("gate-off").hidden = canSignIn;
  if (canSignIn) $("password").focus();
}

function showApp() {
  $("gate").hidden = true;
  $("app").hidden = false;
  selectTab(location.hash.slice(1) in loaders ? location.hash.slice(1) : "activity");
}

$("login").addEventListener("submit", async (event) => {
  event.preventDefault();
  $("login-error").textContent = "";
  try {
    await api("/login", { method: "POST", body: { password: $("password").value } });
    $("password").value = "";
    showApp();
  } catch (error) {
    $("login-error").textContent = error.message;
  }
});

$("logout").addEventListener(
  "click",
  act(async () => {
    await api("/logout", { method: "POST" });
    showGate(true);
  }),
);

// ---- tabs ---------------------------------------------------------------------

const loaders = { activity: loadActivity, users: loadUsers, releases: loadReleases, wallpapers: loadWallpapers, people: loadPeople };
const tabButtons = [...document.querySelectorAll(".tabs button")];

function selectTab(name) {
  for (const button of tabButtons) button.setAttribute("aria-selected", String(button.dataset.tab === name));
  for (const key of Object.keys(loaders)) $("tab-" + key).hidden = key !== name;
  history.replaceState(null, "", "#" + name);
  act(loaders[name])();
}
for (const button of tabButtons) button.addEventListener("click", () => selectTab(button.dataset.tab));
// A link or the Back button that changes only the part after "#".
window.addEventListener("hashchange", () => {
  const name = location.hash.slice(1);
  if (name in loaders && $("tab-" + name).hidden && !$("app").hidden) selectTab(name);
});

// ---- activity -----------------------------------------------------------------

const tip = $("tip");
function showTip(event, text) {
  tip.textContent = text;
  tip.hidden = false;
  const box = tip.getBoundingClientRect();
  tip.style.left = Math.min(window.innerWidth - box.width - 8, Math.max(8, event.clientX - box.width / 2)) + "px";
  tip.style.top = event.clientY - box.height - 12 + "px";
}

// One measure per chart, each on its own scale, so a small number is never
// flattened by a large one beside it.
function chart(title, note, days, key) {
  const peak = Math.max(0, ...days.map((d) => d[key]));
  const max = Math.max(1, peak);
  const bars = el("div", { className: "bars", role: "img" });
  bars.setAttribute("aria-label", `${title}: ${number.format(days.reduce((sum, d) => sum + d[key], 0))} in the last 30 days`);
  for (const d of days) {
    const bar = el("i");
    bar.style.setProperty("--h", (d[key] / max) * 100 + "%");
    if (d[key] > 0) bar.style.setProperty("--min", "2px");
    const text = `${dayLabel(d.day)}: ${number.format(d[key])}`;
    bar.addEventListener("mousemove", (event) => showTip(event, text));
    bar.addEventListener("mouseleave", () => (tip.hidden = true));
    bars.append(bar);
  }
  const axis = el("div", { className: "axis" }, el("span", { textContent: dayLabel(days[0].day) }), el("span", { textContent: `most in a day: ${number.format(peak)}` }), el("span", { textContent: dayLabel(days[days.length - 1].day) }));
  return el("div", { className: "chart" }, el("h3", { textContent: title }), el("p", { textContent: note }), bars, axis);
}

function tile(value, label, small) {
  return el("div", { className: "tile" }, el("b", { textContent: number.format(value) }), el("span", { textContent: label }), el("small", { textContent: small || "" }));
}

async function loadActivity() {
  const data = await api("/overview");
  const days = data.days;
  const sum = (key, last) => days.slice(-last).reduce((total, d) => total + d[key], 0);

  $("signin-off").hidden = data.users.signIn;
  $("tiles").replaceChildren(
    tile(data.users.total, "users", `${number.format(data.users.active7)} did something in 7 days, ${number.format(data.users.active30)} in 30`),
    tile(sum("newUsers", 7), "new users, 7 days", `${number.format(sum("newUsers", 30))} in 30 days`),
    tile(sum("downloads", 7), "downloads, 7 days", `${number.format(sum("downloads", 30))} in 30 days`),
    tile(sum("visits", 7), "page visits, 7 days", `${number.format(sum("visits", 30))} in 30 days`),
    tile(sum("discover", 7), "Discover opened, 7 days", `${number.format(sum("discover", 30))} in 30 days`),
    tile(sum("files", 7), "wallpapers fetched, 7 days", `${number.format(sum("files", 30))} in 30 days`),
  );
  $("charts").replaceChildren(
    chart("Page visits", "The landing page was opened, once per browser tab.", days, "visits"),
    chart("Downloads", "A build of the app was downloaded from the website.", days, "downloads"),
    chart("New users", "People who signed in to the app for the first time.", days, "newUsers"),
    chart("Active users", "Signed-in people who did something in the app that day.", days, "activeUsers"),
    chart("Discover opened", "The app asked for the list of collections.", days, "discover"),
    chart("Wallpapers fetched", "A full file was downloaded by the app.", days, "files"),
  );

  const topList = (list, empty, unit) =>
    list.length
      ? list.map((w) =>
          el("li", {}, el("img", { src: w.thumb, alt: "", loading: "lazy" }), el("span", {}, w.title, " ", el("small", { textContent: w.kind === "image" ? "" : "live" })), el("b", { textContent: number.format(w.count) + unit })),
        )
      : [el("li", { className: "hint", textContent: empty })];
  $("top").replaceChildren(...topList(data.topWallpapers, "Nothing fetched yet.", ""));
  $("top-saved").replaceChildren(...topList(data.topSaved, "Nobody signed in has saved anything yet.", ""));
  $("log").replaceChildren(
    ...(data.events.length
      ? data.events.map((e) => el("li", {}, el("span", {}, e.action, " ", el("small", { textContent: e.detail })), el("time", { textContent: whenLabel(e.at) })))
      : [el("li", { className: "hint", textContent: "Nothing yet." })]),
  );
}

// ---- people -------------------------------------------------------------------

let people = [];
const ticked = new Set();
const peopleBody = $("people").tBodies[0];

function chosen() {
  const shown = people.filter((p) => ticked.has(p.id));
  return shown.length ? shown : people;
}

function syncSelection() {
  const n = people.filter((p) => ticked.has(p.id)).length;
  $("mark-sent").disabled = $("mark-new").disabled = n === 0;
  $("all").checked = n > 0 && n === people.length;
  $("selection").textContent = n
    ? `${number.format(n)} ticked.`
    : "Tick people to mark them. Copy takes the ticked people, or everyone shown. Download is the whole list.";
}

function renderPeople() {
  peopleBody.replaceChildren(
    ...people.map((p) => {
      const box = el("input", { type: "checkbox", checked: ticked.has(p.id) });
      box.setAttribute("aria-label", "Tick " + p.email);
      box.addEventListener("change", () => {
        box.checked ? ticked.add(p.id) : ticked.delete(p.id);
        syncSelection();
      });
      const note = el("button", { type: "button", textContent: p.note || "Add a note", title: "Change the note" });
      note.addEventListener(
        "click",
        act(async () => {
          const next = window.prompt("Note for " + p.email, p.note);
          if (next === null) return;
          await api("/signups/" + p.id, { method: "PATCH", body: { note: next } });
          await loadPeople();
        }),
      );
      const remove = el("button", { type: "button", className: "plain", textContent: "Remove" });
      remove.addEventListener(
        "click",
        act(async () => {
          if (!window.confirm(`Remove ${p.email} from the list? This cannot be undone.`)) return;
          await api("/signups/" + p.id, { method: "DELETE" });
          ticked.delete(p.id);
          await loadPeople();
        }),
      );
      const sent = p.status === "invited";
      return el(
        "tr",
        {},
        el("td", { className: "tick" }, box),
        el("td", { textContent: p.email }),
        el("td", { textContent: whenLabel(p.createdAt) }),
        el("td", {}, p.source === "manual" ? "Added by you" : "Landing page", " ", el("small", { textContent: p.country })),
        el("td", {}, el("span", { className: sent ? "pill sent" : "pill", textContent: sent ? "Sent" : "Not sent", title: sent ? "Marked as sent " + whenLabel(p.invitedAt) : "" })),
        el("td", { className: "note" }, note),
        el("td", {}, remove),
      );
    }),
  );
  $("people-empty").hidden = people.length > 0;
  syncSelection();
}

async function loadPeople() {
  const params = new URLSearchParams();
  if ($("search").value.trim()) params.set("q", $("search").value.trim());
  if ($("status").value) params.set("status", $("status").value);
  const data = await api("/signups?" + params);
  people = data.people;
  $("people-count").textContent =
    `${number.format(people.length)} shown of ${number.format(data.total)}, ${number.format(data.invited)} sent the build` + (data.more ? " (list cut short: search to narrow it)" : "");
  $("csv").href = "/v1/admin/signups.csv" + ($("status").value ? "?status=" + $("status").value : "");
  renderPeople();
}

let searchTimer;
$("search").addEventListener("input", () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(act(loadPeople), 250);
});
$("status").addEventListener("change", act(loadPeople));
$("all").addEventListener("change", () => {
  for (const p of people) $("all").checked ? ticked.add(p.id) : ticked.delete(p.id);
  renderPeople();
});

async function mark(status) {
  const ids = people.filter((p) => ticked.has(p.id)).map((p) => p.id);
  if (!ids.length) return;
  await api("/signups/status", { method: "POST", body: { ids, status } });
  ticked.clear();
  await loadPeople();
}
$("mark-sent").addEventListener("click", act(() => mark("invited")));
$("mark-new").addEventListener("click", act(() => mark("new")));

$("copy").addEventListener(
  "click",
  act(async () => {
    const list = chosen();
    if (!list.length) return;
    await navigator.clipboard.writeText(list.map((p) => p.email).join(", "));
    $("copy").textContent = `Copied ${number.format(list.length)}`;
    setTimeout(() => ($("copy").textContent = "Copy addresses"), 1800);
  }),
);

$("add").addEventListener(
  "submit",
  act(async () => {
    await api("/signups", { method: "POST", body: { email: $("add-email").value, note: $("add-note").value } });
    $("add").reset();
    await loadPeople();
  }),
);

// ---- users --------------------------------------------------------------------

const EVENT_WORDS = {
  app_open: "Opened the app",
  discover_open: "Opened Discover",
  search: "Searched for",
  view: "Looked at",
  save: "Saved",
  set: "Put on the desktop",
  remove: "Removed from the library",
  pack_open: "Opened the collection",
  pack_save: "Saved the collection",
};
const userDialog = $("user-dialog");
$("user-close").addEventListener("click", () => userDialog.close());

async function showUser(u) {
  const data = await api("/users/" + u.id);
  $("user-title").textContent = data.user.email;
  const c = data.counts;
  const part = (word, n) => `${word} ${number.format(n || 0)}`;
  $("user-summary").textContent =
    `Joined ${whenLabel(data.user.createdAt)}, last seen ${whenLabel(data.user.lastSeenAt)}. ` +
    `${part("Saved", c.save)}, ${part("set", c.set)}, ${part("looked at", c.view)}, ${part("searched", c.search)}, ${part("opened the app", c.app_open)}.` +
    (data.more ? ` Showing the newest ${number.format(data.events.length)} things.` : "");
  $("user-events").replaceChildren(
    ...(data.events.length
      ? data.events.map((e) => {
          // What it was done to: a wallpaper or collection by name, a search by
          // its words, the app by its version and system.
          const what = e.kind === "app_open" ? [e.subject, e.detail].filter(Boolean).join(", ") : e.title || e.detail || e.subject;
          const line = el("span", {}, EVENT_WORDS[e.kind] || e.kind, what ? " " : "", what ? el("b", { textContent: what }) : "");
          return el("li", {}, e.thumb ? el("img", { src: e.thumb, alt: "", loading: "lazy" }) : "", line, el("time", { textContent: whenLabel(e.at) }));
        })
      : [el("li", { className: "hint", textContent: "Nothing recorded yet." })]),
  );
  userDialog.showModal();
}

async function loadUsers() {
  const params = new URLSearchParams();
  if ($("user-search").value.trim()) params.set("q", $("user-search").value.trim());
  if ($("user-status").value) params.set("status", $("user-status").value);
  const data = await api("/users?" + params);
  $("user-count").textContent =
    `${number.format(data.users.length)} shown of ${number.format(data.total)}` +
    (data.blocked ? `, ${number.format(data.blocked)} blocked` : "") +
    (data.more ? " (list cut short: search to narrow it)" : "");
  $("user-csv").href = "/v1/admin/users.csv" + ($("user-status").value ? "?status=" + $("user-status").value : "");
  $("users-empty").hidden = data.users.length > 0;
  $("users-empty").textContent = data.total ? "Nobody matches." : data.signIn ? "Nobody has signed in yet." : "Nobody has signed in yet: signing in is switched off on this server (see Activity).";

  $("users").tBodies[0].replaceChildren(
    ...data.users.map((u) => {
      const blocked = u.status === "blocked";
      const open = el("button", { type: "button", className: "link", textContent: u.email, title: "See what they did" });
      open.addEventListener("click", act(() => showUser(u)));
      const note = el("button", { type: "button", textContent: u.note || "Add a note", title: "Change the note" });
      note.addEventListener(
        "click",
        act(async () => {
          const next = window.prompt("Note for " + u.email, u.note);
          if (next === null) return;
          await api("/users/" + u.id, { method: "PATCH", body: { note: next } });
          await loadUsers();
        }),
      );
      const block = el("button", { type: "button", className: "plain", textContent: blocked ? "Unblock" : "Block" });
      block.addEventListener(
        "click",
        act(async () => {
          if (!blocked && !window.confirm(`Block ${u.email}? They are signed out everywhere and cannot sign in again until you unblock them.`)) return;
          await api("/users/" + u.id, { method: "PATCH", body: { status: blocked ? "active" : "blocked" } });
          await loadUsers();
        }),
      );
      const remove = el("button", { type: "button", className: "plain", textContent: "Remove" });
      remove.addEventListener(
        "click",
        act(async () => {
          if (!window.confirm(`Remove ${u.email} and everything recorded about them? This cannot be undone.`)) return;
          await api("/users/" + u.id, { method: "DELETE" });
          await loadUsers();
        }),
      );
      return el(
        "tr",
        {},
        el("td", {}, open),
        el("td", { textContent: whenLabel(u.createdAt) }),
        el("td", { textContent: whenLabel(u.lastSeenAt) }),
        el("td", { textContent: u.country }),
        el("td", { className: "num", textContent: number.format(u.events) }),
        el("td", {}, el("span", { className: blocked ? "pill bad" : "pill", textContent: blocked ? "Blocked" : "Can sign in" })),
        el("td", { className: "note" }, note),
        el("td", {}, block, remove),
      );
    }),
  );
}

let userSearchTimer;
$("user-search").addEventListener("input", () => {
  clearTimeout(userSearchTimer);
  userSearchTimer = setTimeout(act(loadUsers), 250);
});
$("user-status").addEventListener("change", act(loadUsers));

// ---- releases -----------------------------------------------------------------

// Which system a file is probably for, from its name. The owner can change it.
function guessPlatform(name) {
  const n = name.toLowerCase();
  const arm = /aarch64|arm64/.test(n);
  if (/\.(dmg|pkg)$/.test(n) || /mac|darwin/.test(n)) return /universal/.test(n) ? "mac-universal" : /x64|x86_64|intel/.test(n) ? "mac-x64" : "mac-arm64";
  if (/\.(exe|msi)$/.test(n)) return arm ? "windows-arm64" : "windows-x64";
  if (/\.(appimage|deb|rpm|tar\.gz)$/.test(n)) return arm ? "linux-arm64" : "linux-x64";
  return "";
}

function releaseCard(r, data) {
  const published = r.publishedAt !== null;
  const reload = () => loadReleases();

  const state = r.latest ? "On the website" : published ? "Published, replaced by a newer one" : "Draft";
  const head = el(
    "header",
    {},
    el("h2", {}, r.version, " ", el("span", { className: "pill", textContent: r.channel === "stable" ? "Stable" : "Beta" })),
    el("span", { className: r.latest ? "pill sent" : "pill", textContent: state }),
    el("span", { className: "grow hint", textContent: published ? "Published " + whenLabel(r.publishedAt) : "Started " + whenLabel(r.createdAt) }),
  );

  const publish = el("button", { type: "button", className: published ? "" : "primary", textContent: published ? "Take back" : "Publish" });
  publish.disabled = !published && r.builds.length === 0;
  publish.title = publish.disabled ? "Add at least one file first" : "";
  publish.addEventListener(
    "click",
    act(async () => {
      const question = published
        ? `Take ${r.version} back? The website stops offering it` + (r.latest ? " and offers the release published before it, if there is one." : ".")
        : `Publish ${r.version}? The website will offer it to everyone straight away.`;
      if (!window.confirm(question)) return;
      await api("/releases/" + r.id, { method: "PATCH", body: { published: !published } });
      await reload();
    }),
  );
  const channel = el("button", { type: "button", textContent: r.channel === "stable" ? "Call it beta" : "Call it stable" });
  channel.addEventListener(
    "click",
    act(async () => {
      await api("/releases/" + r.id, { method: "PATCH", body: { channel: r.channel === "stable" ? "beta" : "stable" } });
      await reload();
    }),
  );
  const notes = el("button", { type: "button", textContent: r.notes ? "Change the notes" : "Add notes" });
  notes.addEventListener(
    "click",
    act(async () => {
      const next = window.prompt("What is new in " + r.version + "? This is shown on the website.", r.notes);
      if (next === null) return;
      await api("/releases/" + r.id, { method: "PATCH", body: { notes: next } });
      await reload();
    }),
  );
  const remove = el("button", { type: "button", className: "plain", textContent: "Delete release" });
  remove.addEventListener(
    "click",
    act(async () => {
      if (!window.confirm(`Delete ${r.version} and its ${r.builds.length} file(s)? This cannot be undone.`)) return;
      await api("/releases/" + r.id, { method: "DELETE" });
      await reload();
    }),
  );

  const rows = r.builds.map((b) => {
    const drop = el("button", { type: "button", className: "plain", textContent: "Remove" });
    drop.addEventListener(
      "click",
      act(async () => {
        if (!window.confirm(`Remove ${b.filename}? The file is deleted.`)) return;
        await api("/builds/" + b.id, { method: "DELETE" });
        await reload();
      }),
    );
    return el(
      "tr",
      {},
      el("td", { textContent: b.label }),
      el("td", {}, published ? el("a", { href: b.url, textContent: b.filename }) : b.filename),
      el("td", { className: "num", textContent: sizeLabel(b.bytes) }),
      el("td", {}, el("code", { textContent: b.sha256.slice(0, 16) + "…", title: "SHA-256: " + b.sha256 })),
      el("td", { className: "num", textContent: number.format(b.downloads30) }),
      el("td", { className: "num", textContent: number.format(b.downloadsAll) }),
      el("td", {}, drop),
    );
  });
  const table = el(
    "table",
    {},
    el("thead", {}, el("tr", {}, ...["System", "File", "Size", "Fingerprint", "Downloads, 30 days", "All time", ""].map((t, i) => el("th", { className: i === 2 || i > 3 ? "num" : "", textContent: t })))),
    el("tbody", {}, ...rows),
  );

  // Adding a file: choose it, check the system it is for, upload.
  const file = el("input", { type: "file" });
  file.setAttribute("aria-label", "File for " + r.version);
  const system = el("select", {}, el("option", { value: "", textContent: "Which system?" }), ...data.platforms.map((p) => el("option", { value: p.id, textContent: p.label })));
  system.setAttribute("aria-label", "System the file is for");
  const send = el("button", { type: "submit", textContent: "Upload" });
  const progress = el("span", { className: "grow hint" });
  file.addEventListener("change", () => {
    if (file.files[0]) system.value = guessPlatform(file.files[0].name);
  });
  const upload = el("form", { className: "bar" }, file, system, send, progress);
  upload.addEventListener(
    "submit",
    act(async () => {
      const chosen = file.files[0];
      if (!chosen) throw new Error("Choose the file to upload.");
      if (!system.value) throw new Error("Choose which system the file is for.");
      if (chosen.size > data.maxBuildBytes) throw new Error(`That file is ${sizeLabel(chosen.size)}; a build can be at most ${sizeLabel(data.maxBuildBytes)}.`);
      send.disabled = true;
      progress.textContent = `Uploading ${sizeLabel(chosen.size)}…`;
      try {
        await api(`/releases/${r.id}/builds?` + new URLSearchParams({ platform: system.value, filename: chosen.name }), { method: "POST", file: chosen });
      } finally {
        send.disabled = false;
        progress.textContent = "";
      }
      await reload();
    }),
  );
  upload.hidden = !data.enabled;

  return el(
    "article",
    { className: "release" },
    head,
    r.notes ? el("p", { className: "notes", textContent: r.notes }) : "",
    r.builds.length ? el("div", { className: "scroll" }, table) : el("p", { className: "hint", textContent: "No files yet." }),
    upload,
    el("div", { className: "bar" }, publish, channel, notes, el("span", { className: "grow" }), remove),
  );
}

async function loadReleases() {
  const data = await api("/releases");
  $("releases-off").hidden = data.enabled;
  $("releases-empty").hidden = data.releases.length > 0;
  $("releases").replaceChildren(...data.releases.map((r) => releaseCard(r, data)));
}

$("release-new").addEventListener(
  "submit",
  act(async () => {
    await api("/releases", { method: "POST", body: { version: $("release-version").value, channel: $("release-channel").value, notes: $("release-notes").value } });
    $("release-new").reset();
    await loadReleases();
  }),
);

// ---- wallpapers ---------------------------------------------------------------

let items = [];
const sort = { key: "fetches30", down: true };
const sortButtons = [...document.querySelectorAll("#items [data-sort]")];

function renderItems() {
  const { key, down } = sort;
  const sorted = [...items].sort((a, b) => {
    const order = typeof a[key] === "number" ? a[key] - b[key] : String(a[key]).localeCompare(String(b[key]));
    return (down ? -order : order) || a.title.localeCompare(b.title);
  });
  for (const button of sortButtons) {
    if (button.dataset.sort === key) button.setAttribute("aria-sort", down ? "descending" : "ascending");
    else button.removeAttribute("aria-sort");
  }
  $("items").tBodies[0].replaceChildren(
    ...sorted.map((w) =>
      el(
        "tr",
        {},
        el("td", {}, el("img", { src: w.thumb, alt: "", loading: "lazy" })),
        el("td", {}, w.title, " ", el("small", { textContent: `${w.width} × ${w.height}` })),
        el("td", { textContent: w.kind === "image" ? "Still" : w.kind === "scene" ? "Live scene" : "Video" }),
        el("td", { textContent: w.category }),
        el("td", { className: "num", textContent: sizeLabel(w.storedBytes) }),
        el("td", { className: "num", textContent: number.format(w.fetches7) }),
        el("td", { className: "num", textContent: number.format(w.fetches30) }),
        el("td", { className: "num", textContent: number.format(w.fetchesAll) }),
        el("td", { className: "num", textContent: number.format(w.savedBy) }),
        el("td", { className: "num", textContent: number.format(w.setBy) }),
      ),
    ),
  );
}
for (const button of sortButtons) {
  button.addEventListener("click", () => {
    const key = button.dataset.sort;
    const text = key === "title" || key === "kind" || key === "category";
    if (sort.key === key) sort.down = !sort.down;
    else Object.assign(sort, { key, down: !text });
    renderItems();
  });
}

async function loadWallpapers() {
  const data = await api("/wallpapers");
  items = data.items;
  $("storage").textContent =
    `${number.format(items.length)} wallpapers using ${sizeLabel(data.storedBytes)}` + (data.capBytes ? ` of the ${sizeLabel(data.capBytes)} storage budget.` : ".");
  $("packs").tBodies[0].replaceChildren(
    ...data.packs.map((p) =>
      el("tr", {}, el("td", { textContent: p.name }), el("td", { className: "num", textContent: number.format(p.count) }), el("td", { className: "num", textContent: number.format(p.opens30) })),
    ),
  );
  renderItems();
}

// ---- start --------------------------------------------------------------------

(async () => {
  try {
    const session = await api("/session");
    if (session.signedIn) showApp();
    else showGate(session.enabled);
  } catch (error) {
    $("gate").hidden = false;
    $("gate-off").hidden = false;
    $("gate-off").textContent = error.message;
  }
})();
