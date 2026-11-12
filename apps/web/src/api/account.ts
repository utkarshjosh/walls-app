// Signing in, and telling the catalog service what a signed-in person does.
//
// Signing in is optional and has no password: the service emails a six-digit
// code and the person types it back (apps/api, "Signing in"). The app then
// holds a token, kept in this window's local storage, and sends it with the
// few requests below. Nothing here is needed to use the app.
//
// While someone is signed in, `track` reports what they do with catalog
// wallpapers: opened, saved, set, removed, searched for. That is what Discover
// will suggest from. Signed out, `track` does nothing at all. Wallpapers the
// person added themselves are never reported: callers pass catalog ids only.

import { useSyncExternalStore } from "react";
import { API_BASE } from "./client";

export interface Account {
  email: string;
}

export class AccountError extends Error {
  constructor(
    message: string,
    /** The service's error code, or "offline" when it could not be reached. */
    readonly code: string,
  ) {
    super(message);
  }
}

const TOKEN_KEY = "still.account.token";
const EMAIL_KEY = "still.account.email";

let token: string | null = null;
let current: Account | null = null;
try {
  const storedToken = localStorage.getItem(TOKEN_KEY);
  const storedEmail = localStorage.getItem(EMAIL_KEY);
  if (storedToken && storedEmail) {
    token = storedToken;
    current = { email: storedEmail };
  }
} catch {
  // Storage is unavailable: the person is simply signed out each launch.
}

const listeners = new Set<() => void>();

function remember(next: Account | null, nextToken: string | null) {
  current = next;
  token = nextToken;
  try {
    if (next && nextToken) {
      localStorage.setItem(TOKEN_KEY, nextToken);
      localStorage.setItem(EMAIL_KEY, next.email);
    } else {
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(EMAIL_KEY);
    }
  } catch {
    // Kept in memory for this launch only.
  }
  if (!next) queue.length = 0;
  for (const listener of listeners) listener();
}

async function call<T>(method: string, path: string, body?: unknown, signedIn = false): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method,
      headers: {
        accept: "application/json",
        ...(body === undefined ? {} : { "content-type": "application/json" }),
        ...(signedIn && token ? { authorization: `Bearer ${token}` } : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new AccountError("The server can't be reached right now. Check your connection.", "offline");
  }
  if (response.status === 204) return undefined as T;
  const data = (await response.json().catch(() => null)) as { error?: string; message?: string } | null;
  if (!response.ok) {
    // The sign-in ended somewhere else (signed out, removed, or blocked).
    if (signedIn && (response.status === 401 || response.status === 403)) remember(null, null);
    throw new AccountError(data?.message ?? "That did not work. Try again in a moment.", data?.error ?? "unknown");
  }
  return data as T;
}

/** What is signing in, shown to the owner beside each sign-in. */
const CLIENT = `Still ${__APP_VERSION__} ${"__TAURI_INTERNALS__" in window ? "Mac app" : "browser preview"}`;

let availability: Promise<boolean> | null = null;

export const account = {
  /** The signed-in person, or null. */
  get: () => current,

  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => void listeners.delete(listener);
  },

  /** Whether this catalog service offers signing in at all. Asked once. */
  available(): Promise<boolean> {
    availability ??= call<{ signIn?: boolean }>("GET", "/v1/auth/status")
      .then((status) => status.signIn === true)
      .catch(() => {
        availability = null; // offline now; ask again next time
        return false;
      });
    return availability;
  },

  /** Step one: have a code emailed to the address. */
  async start(email: string): Promise<void> {
    await call("POST", "/v1/auth/start", { email });
  },

  /** Step two: trade the code for a sign-in. */
  async verify(email: string, code: string): Promise<Account> {
    const result = await call<{ token: string; user: Account }>("POST", "/v1/auth/verify", { email, code, client: CLIENT });
    remember({ email: result.user.email }, result.token);
    track("app_open", __APP_VERSION__, CLIENT);
    return result.user;
  },

  /** Checks at launch that a remembered sign-in still stands. */
  async refresh(): Promise<void> {
    if (!token) return;
    try {
      const me = await call<Account>("GET", "/v1/me", undefined, true);
      if (me.email !== current?.email) remember({ email: me.email }, token);
    } catch {
      // Offline: keep what we have. Ended elsewhere: `call` already cleared it.
    }
  },

  /** Stops reporting from now on. What was already recorded stays. */
  async signOut(): Promise<void> {
    await flush();
    await call("POST", "/v1/auth/signout", undefined, true).catch(() => {});
    remember(null, null);
  },

  /** Deletes the account and everything recorded with it. */
  async remove(): Promise<void> {
    queue.length = 0;
    await call("DELETE", "/v1/me", undefined, true);
    remember(null, null);
  },
};

/** The signed-in person, kept current as they sign in and out. */
export function useAccount(): Account | null {
  return useSyncExternalStore(account.subscribe, account.get);
}

// ── Reporting ──────────────────────────────────────────────────────────────

/** The kinds the service accepts (apps/api/internal/records/users.go). */
export type EventKind =
  | "app_open"
  | "discover_open"
  | "search"
  | "view"
  | "save"
  | "set"
  | "remove"
  | "pack_open"
  | "pack_save";

interface QueuedEvent {
  kind: EventKind;
  subject: string;
  detail: string;
  at: string;
}

const queue: QueuedEvent[] = [];
const MAX_QUEUED = 200; // an app left offline for days does not grow without limit
const BATCH = 50; // the most the service takes in one request
let timer: number | undefined;

/**
 * Notes something a signed-in person did. `subject` is a catalog wallpaper or
 * collection id; `detail` is free text (a search). Does nothing when signed
 * out. Reports are sent a few seconds later, several at a time.
 */
export function track(kind: EventKind, subject = "", detail = ""): void {
  if (!token) return;
  queue.push({ kind, subject, detail, at: new Date().toISOString() });
  if (queue.length > MAX_QUEUED) queue.splice(0, queue.length - MAX_QUEUED);
  if (queue.length >= 20) void flush();
  else timer ??= window.setTimeout(() => void flush(), 3000);
}

async function flush(closing = false): Promise<void> {
  window.clearTimeout(timer);
  timer = undefined;
  if (!token || queue.length === 0) return;
  const batch = queue.splice(0, BATCH);
  try {
    const response = await fetch(`${API_BASE}/v1/events`, {
      method: "POST",
      // Lets the request finish when the window is closing.
      keepalive: closing,
      headers: { "content-type": "application/json", authorization: `Bearer ${token}` },
      body: JSON.stringify({ events: batch }),
    });
    if (response.status === 401 || response.status === 403) remember(null, null);
    // Busy or broken for the moment: keep them for the next try.
    else if (response.status === 429 || response.status >= 500) queue.unshift(...batch);
  } catch {
    queue.unshift(...batch); // offline
  }
  if (queue.length && token) timer ??= window.setTimeout(() => void flush(), 60_000);
}

if (typeof document !== "undefined") {
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) void flush(true);
  });
  window.addEventListener("pagehide", () => void flush(true));
}
