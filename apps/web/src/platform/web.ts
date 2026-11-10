import { localDb } from "./db";
import type {
  ApplyRequest,
  DisplayInfo,
  Platform,
  RotationCommand,
  RotationPlan,
  RotationStatus,
  StoredFile,
} from "./types";
import { fitsDisplay, placementFor } from "@/lib/fit";
import {
  advance,
  advanceSlots,
  back,
  emptyEngine,
  reconcile,
  show,
  type EngineState,
} from "@/lib/rotation";
import { nextChange } from "@/lib/schedule";

// The browser host. A web page cannot change the real desktop, so everything
// here is an in-app preview: files live in IndexedDB, rotation runs on a page
// timer, and the "displays" are either this screen or a simulated arrangement
// chosen from the preview tools (to try the multi-display features without the
// hardware).

const DISPLAY_KEY = "still.preview.displays";

/** Arrangements the preview tools can pretend are connected. */
export type SimulatedLayout = "1" | "2" | "2v" | "3";
const LAYOUTS: readonly SimulatedLayout[] = ["1", "2", "2v", "3"];

function simulatedDisplays(layout: SimulatedLayout): DisplayInfo[] {
  const alone = layout === "1";
  const builtIn: DisplayInfo = {
    id: "preview-1",
    name: "Built-in Display",
    x: 0,
    y: 0,
    width: alone ? window.screen.width : 1512,
    height: alone ? window.screen.height : 982,
    scale: window.devicePixelRatio >= 2 || !alone ? 2 : 1,
    primary: true,
  };
  if (alone) return [builtIn];
  const studio: DisplayInfo = {
    id: "preview-2",
    name: "Studio Display",
    // Beside the laptop, or centred above it for the stacked arrangement.
    x: layout === "2v" ? -524 : 1512,
    y: layout === "2v" ? -1440 : -458,
    width: 2560,
    height: 1440,
    scale: 2,
    primary: false,
  };
  if (layout !== "3") return [builtIn, studio];
  const side: DisplayInfo = {
    id: "preview-3",
    name: "LG UltraFine",
    x: -1920,
    y: -98,
    width: 1920,
    height: 1080,
    scale: 1,
    primary: false,
  };
  return [side, builtIn, studio];
}

function readLayout(): SimulatedLayout {
  try {
    const value = localStorage.getItem(DISPLAY_KEY) as SimulatedLayout | null;
    return value && LAYOUTS.includes(value) ? value : "1";
  } catch {
    return "1";
  }
}

function leftToRight(displays: DisplayInfo[]): DisplayInfo[] {
  return [...displays].sort((a, b) => a.x - b.x || a.y - b.y);
}

export interface WebPlatform extends Platform {
  /** Preview tools only: pretend this arrangement of displays is attached. */
  setSimulatedLayout(layout: SimulatedLayout): void;
  simulatedLayout(): SimulatedLayout;
}

export function createWebPlatform(): WebPlatform {
  const objectUrls = new Map<string, string>();
  const displayListeners = new Set<(displays: DisplayInfo[]) => void>();
  const rotationListeners = new Set<(status: RotationStatus) => void>();

  let layout = readLayout();
  let plan: RotationPlan | null = null;
  let engine: EngineState = emptyEngine();
  let nextAt: number | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;

  const displays = () => simulatedDisplays(layout);
  const frameSize = () => (plan?.displayMode === "each" ? displays().length : 1);
  const ids = () => plan?.items.map((item) => item.id) ?? [];

  function status(): RotationStatus {
    const ordered = leftToRight(displays());
    const perDisplay = plan?.displayMode === "each" && ordered.length > 1;
    return {
      paused: plan?.paused ?? true,
      current: perDisplay
        ? engine.current
            .map((itemId, index) => ({ displayId: ordered[index % ordered.length]!.id, itemId }))
            // "" marks a display the app has put nothing on.
            .filter((entry) => entry.itemId !== "")
        : engine.current
            .filter((itemId) => itemId !== "")
            .slice(0, 1)
            .map((itemId) => ({ displayId: null, itemId })),
      nextChangeAt: nextAt,
      canGoPrevious: engine.history.length > 0,
    };
  }

  function emit() {
    const snapshot = status();
    for (const listener of rotationListeners) listener(snapshot);
  }

  /** For each display (left to right), the wallpapers large enough for it. */
  function allowedPerDisplay(): Set<string>[] {
    return leftToRight(displays()).map(
      (display) =>
        new Set(
          (plan?.items ?? []).filter((item) => fitsDisplay(item, display)).map((item) => item.id),
        ),
    );
  }

  // Automatic changes only use wallpapers that are large enough for where they
  // would be shown (src/lib/fit.ts), exactly as the Mac app's native core does.
  function step() {
    if (!plan) return;
    const connected = displays();
    const mode = connected.length > 1 ? plan.displayMode : "same";
    if (mode === "each") {
      engine = advanceSlots(engine, ids(), allowedPerDisplay(), { shuffle: plan.shuffle });
      return;
    }
    const eligible = plan.items
      .filter((item) => placementFor(item, connected, mode) !== "skip")
      .map((item) => item.id);
    engine = advance(engine, eligible, { shuffle: plan.shuffle });
  }

  function arm() {
    clearTimeout(timer);
    const due = plan
      ? nextChange({ schedule: plan.schedule, paused: plan.paused, itemCount: plan.items.length })
      : null;
    nextAt = due ? due.getTime() : null;
    if (nextAt === null) return;
    // Re-check at least once a minute so a sleeping laptop or a clock change
    // cannot leave a stale long timer behind.
    const wait = Math.min(Math.max(nextAt - Date.now(), 0), 60_000);
    timer = setTimeout(() => {
      if (nextAt !== null && Date.now() >= nextAt) {
        step();
        arm();
        emit();
      } else {
        arm();
      }
    }, wait);
  }

  /** Keep the frame the right size when the display mode or count changes. */
  function resizeFrame() {
    if (!plan || engine.current.length === 0) return;
    const wanted = frameSize();
    if (engine.current.length > wanted) {
      const kept = engine.current.filter((id) => id !== "").slice(0, wanted);
      engine = { ...engine, current: kept };
    } else if (engine.current.length < wanted) {
      // Give the other displays wallpapers that are large enough for them;
      // a display with none keeps what it has ("").
      const allowed = allowedPerDisplay();
      const frame = [...engine.current];
      for (let slot = frame.length; slot < wanted; slot++) {
        const choices = ids().filter((id) => allowed[slot]?.has(id));
        frame.push(choices.find((id) => !frame.includes(id)) ?? choices[0] ?? "");
      }
      engine = { ...engine, current: frame };
    }
  }

  async function fileRecord(name: string): Promise<StoredFile | null> {
    const blob = (await (await localDb()).get("files", name)) as Blob | undefined;
    if (!blob) return null;
    let url = objectUrls.get(name);
    if (!url) {
      url = URL.createObjectURL(blob);
      objectUrls.set(name, url);
    }
    return { name, path: name, url, bytes: blob.size };
  }

  return {
    kind: "web",
    canSetWallpaper: false,

    async displays() {
      return displays();
    },
    onDisplaysChanged(listener) {
      displayListeners.add(listener);
      return () => displayListeners.delete(listener);
    },
    setSimulatedLayout(next) {
      layout = next;
      try {
        localStorage.setItem(DISPLAY_KEY, layout);
      } catch {
        /* preview-only preference; fine to lose */
      }
      resizeFrame();
      for (const listener of displayListeners) listener(displays());
      emit();
    },
    simulatedLayout: () => layout,

    async storeFile(name, data) {
      const db = await localDb();
      await db.put("files", data, name);
      const stale = objectUrls.get(name);
      if (stale) {
        URL.revokeObjectURL(stale);
        objectUrls.delete(name);
      }
      return (await fileRecord(name))!;
    },
    async downloadFile(name, url) {
      // A browser tab just points at the remote file; nothing is copied.
      return { name, path: url, url, bytes: 0 };
    },
    async removeFiles(names) {
      const db = await localDb();
      const tx = db.transaction("files", "readwrite");
      await Promise.all(names.map((name) => tx.store.delete(name)));
      await tx.done;
      for (const name of names) {
        const url = objectUrls.get(name);
        if (url) URL.revokeObjectURL(url);
        objectUrls.delete(name);
      }
    },
    async resolveFiles(names) {
      const found: Record<string, StoredFile> = {};
      await Promise.all(
        names.map(async (name) => {
          const record = await fileRecord(name);
          if (record) found[name] = record;
        }),
      );
      return found;
    },

    async apply(request: ApplyRequest) {
      const picked = request.items.map((item) => item.id);
      if (picked.length === 0) return status();
      const ordered = leftToRight(displays());
      const slot = request.displayId
        ? ordered.findIndex((display) => display.id === request.displayId)
        : -1;
      if (slot >= 0 && plan?.displayMode === "each" && ordered.length > 1) {
        // Only one display changes; the others keep what they show.
        resizeFrame();
        // A display the app has put nothing on stays a gap, as in the Mac app.
        const frame = ordered.map((_, index) => engine.current[index] ?? "");
        frame[slot] = picked[0]!;
        engine = show(engine, frame);
      } else {
        const size = request.mode === "each" ? ordered.length : 1;
        engine = show(
          engine,
          Array.from({ length: size }, (_, index) => picked[index % picked.length]!),
        );
      }
      emit();
      return status();
    },
    async syncRotation(next: RotationPlan) {
      plan = next;
      engine = reconcile(engine, ids());
      resizeFrame();
      arm();
      emit();
      return status();
    },
    async rotationCommand(command: RotationCommand) {
      if (!plan) return status();
      if (command === "next") step();
      if (command === "previous") engine = back(engine) ?? engine;
      if (command === "pause") plan = { ...plan, paused: true };
      if (command === "resume") {
        plan = { ...plan, paused: false };
        if (engine.current.length === 0) step();
      }
      arm();
      emit();
      return status();
    },
    async rotationStatus() {
      return status();
    },
    onRotationChanged(listener) {
      rotationListeners.add(listener);
      return () => rotationListeners.delete(listener);
    },
  };
}
