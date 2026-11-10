import { useEffect, useId, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Check, Pause, Play, SkipBack, SkipForward } from "lucide-react";
import { canSpan, fitsDisplay, placementFor, rotationUse } from "@/lib/fit";
import {
  MAX_CUSTOM_MINUTES,
  MIN_CUSTOM_MINUTES,
  clampCustomMinutes,
  describeWait,
  type IntervalPreset,
} from "@/lib/schedule";
import { cn, plural } from "@/lib/utils";
import type { DisplayInfo, DisplayMode } from "@/platform/types";
import { useLibrary } from "@/state/store";
import { isLive, playableRef, type Collection, type Wallpaper } from "@/state/model";
import {
  DisplayPreview,
  ModeGlyph,
  orderedDisplays,
  type PreviewPicture,
} from "@/components/DisplayPreview";
import { Picture } from "@/components/Picture";
import { Button } from "@/components/ui/button";
import {
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
} from "@/components/ui/form";
import { WallpaperPicker } from "@/components/WallpaperPicker";
import { PageHeader } from "./Library";

const INTERVALS: { value: IntervalPreset; label: string }[] = [
  { value: "15m", label: "Every 15 minutes" },
  { value: "30m", label: "Every 30 minutes" },
  { value: "hour", label: "Every hour" },
  { value: "three", label: "Every 3 hours" },
  { value: "day", label: "Once a day" },
  { value: "custom", label: "Custom" },
];

const MODES: { value: DisplayMode; label: string; hint: string }[] = [
  { value: "same", label: "Same on each", hint: "Every display shows the same wallpaper." },
  {
    value: "span",
    label: "One across all",
    hint: "One wallpaper stretches across your displays when it is large enough.",
  },
  {
    value: "each",
    label: "Different on each",
    hint: "Each display gets its own wallpaper. Point at a display above to choose one yourself.",
  },
];

export function RotationPage({
  onOpenLibrary,
  onOpenDiscover,
}: {
  onOpenLibrary: () => void;
  onOpenDiscover: () => void;
}) {
  const library = useLibrary();
  const { state, status, displays, displayMode, urlOf, canSetWallpaper, rotationWallpapers } =
    library;
  const { settings } = state;
  const [working, setWorking] = useState(false);
  const [choosingFor, setChoosingFor] = useState<DisplayInfo | null>(null);
  const now = useClock(status.nextChangeAt !== null);
  const multi = displays.length > 1;

  const run = async (name: Parameters<typeof library.command>[0]) => {
    setWorking(true);
    try {
      await library.command(name);
    } finally {
      setWorking(false);
    }
  };

  // ── What is on the desktop ──────────────────────────────────────────────
  const shown = status.current
    .map((entry) => ({ displayId: entry.displayId, wallpaper: state.wallpapers[entry.itemId] }))
    .filter((entry): entry is { displayId: string | null; wallpaper: Wallpaper } =>
      Boolean(entry.wallpaper),
    );
  const started = shown.length > 0;
  const perDisplay = shown.some((entry) => entry.displayId !== null);

  // Only wallpapers large enough for these displays take part (src/lib/fit.ts).
  const use = rotationUse(rotationWallpapers, displays, displayMode);
  const noneChosen = rotationWallpapers.length === 0;
  const empty = use.spanned + use.shown === 0;
  const displayLabel = multi ? "your displays" : (displays[0]?.name ?? "this display");

  const pictureOf = (wallpaper: Wallpaper): PreviewPicture => {
    // A scene runs in the stage itself, so the stage shows what the desktop shows.
    const live = wallpaper.kind === "scene" ? urlOf(playableRef(wallpaper)) : undefined;
    // A clock's still shows a frozen time; do not flash it under the running clock.
    const frozen = live && wallpaper.tags?.includes("clock");
    return {
      src: frozen
        ? undefined
        : urlOf(wallpaper.kind === "image" ? wallpaper.full : (wallpaper.poster ?? wallpaper.thumb)),
      color: wallpaper.color,
      live,
    };
  };

  // Before anything has been shown, the stage previews what starting would put up.
  let layout: DisplayMode = "same";
  let picture: PreviewPicture | undefined;
  const pictureByDisplay: Record<string, PreviewPicture> = {};
  if (started) {
    if (perDisplay) {
      layout = "each";
      for (const entry of shown)
        if (entry.displayId) pictureByDisplay[entry.displayId] = pictureOf(entry.wallpaper);
    } else {
      const wallpaper = shown[0]!.wallpaper;
      // "One across all" only stretches a picture that has the pixels for it.
      layout = displayMode === "span" && canSpan(wallpaper, displays) ? "span" : "same";
      picture = pictureOf(wallpaper);
    }
  } else if (displayMode === "each" && multi) {
    layout = "each";
    const used = new Set<string>();
    for (const display of orderedDisplays(displays)) {
      const fitting = rotationWallpapers.filter((wallpaper) => fitsDisplay(wallpaper, display));
      const first = fitting.find((wallpaper) => !used.has(wallpaper.id)) ?? fitting[0];
      if (first) {
        used.add(first.id);
        pictureByDisplay[display.id] = pictureOf(first);
      }
    }
  } else {
    const mode = displayMode === "span" ? "span" : "same";
    const first = rotationWallpapers.find(
      (wallpaper) => placementFor(wallpaper, displays, mode) !== "skip",
    );
    if (first) {
      layout = mode === "span" && canSpan(first, displays) ? "span" : "same";
      picture = pictureOf(first);
    }
  }
  const hasPicture = Boolean(picture) || Object.keys(pictureByDisplay).length > 0;

  const titles = [...new Set(shown.map((entry) => entry.wallpaper.title))];
  const nextAt = status.nextChangeAt ? new Date(status.nextChangeAt) : null;
  const hasLive =
    rotationWallpapers.some(isLive) || shown.some((entry) => isLive(entry.wallpaper));

  const statusLine = noneChosen
    ? "No collection is turned on."
    : empty
      ? "No wallpaper is large enough to show."
      : settings.paused
        ? started
          ? "Paused"
          : "Rotation is off"
        : nextAt
          ? `Next change at ${nextAt.toLocaleTimeString(undefined, {
              hour: "numeric",
              minute: "2-digit",
            })}, ${describeWait(nextAt, now)}`
          : "";

  return (
    <div className="scroll-area h-full">
      <div className="mx-auto max-w-[1120px] px-8 pt-7 pb-16">
        <PageHeader title="Rotation" />

        {/* ── The stage: always the same size, whatever the arrangement ── */}
        <section
          aria-label="Now showing"
          className="relative h-[clamp(220px,36vh,330px)] rounded-panel bg-surface p-6"
        >
          {displays.length > 0 && hasPicture ? (
            <DisplayPreview
              contain
              displays={displays}
              layout={layout}
              picture={picture}
              pictureByDisplay={pictureByDisplay}
              labels={multi}
              className={cn("transition-opacity duration-200", !started && "opacity-40")}
              action={
                // With a wallpaper per display, each display can be given one by hand.
                started && multi && displayMode === "each"
                  ? (display) => (
                      <Button variant="glass" size="sm" onClick={() => setChoosingFor(display)}>
                        Choose
                        <span className="sr-only"> a wallpaper for {display.name}</span>
                      </Button>
                    )
                  : undefined
              }
            />
          ) : (
            <div className="flex size-full flex-col items-center justify-center text-center">
              {noneChosen ? (
                <p className="max-w-xs text-sm text-fg-muted">
                  Turn on a collection below and its wallpapers take turns on your desktop.
                </p>
              ) : (
                <>
                  <p className="font-medium">Nothing here is large enough for {displayLabel}</p>
                  <p className="mt-1 max-w-sm text-sm text-fg-muted">
                    These wallpapers would look blurry, so they are not used.
                  </p>
                  <Button className="mt-4" onClick={onOpenDiscover}>
                    Find larger ones in Discover
                  </Button>
                </>
              )}
            </div>
          )}
          {!started && !empty && hasPicture && (
            <div className="absolute inset-0 flex items-center justify-center">
              {/* Nothing is on the desktop yet. If rotation is already running,
                  the first change would otherwise wait for the next scheduled time. */}
              <Button
                variant="primary"
                size="lg"
                disabled={working}
                onClick={() => run(settings.paused ? "resume" : "next")}
              >
                <Play />
                {settings.paused ? "Start rotation" : "Show the first wallpaper"}
              </Button>
            </div>
          )}
        </section>

        {/* ── Transport and timing, on one line ── */}
        <div className="mt-4 flex flex-wrap items-center justify-between gap-x-8 gap-y-4">
          <div className="flex min-w-0 items-center gap-4">
            <div className="flex shrink-0 items-center gap-1">
              <Button
                variant="ghost"
                size="icon"
                aria-label="Previous wallpaper"
                title="Previous wallpaper"
                disabled={working || !status.canGoPrevious}
                onClick={() => run("previous")}
              >
                <SkipBack />
              </Button>
              <Button
                size="icon"
                className="size-10 rounded-full"
                aria-label={settings.paused ? "Resume rotation" : "Pause rotation"}
                title={settings.paused ? "Resume rotation" : "Pause rotation"}
                disabled={working || empty}
                onClick={() => run(settings.paused ? "resume" : "pause")}
              >
                {settings.paused ? <Play /> : <Pause />}
              </Button>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Next wallpaper"
                title="Next wallpaper"
                disabled={working || empty}
                onClick={() => run("next")}
              >
                <SkipForward />
              </Button>
            </div>
            <div className="min-w-0 text-sm">
              {started && <p className="truncate font-medium">{titles.join(" and ")}</p>}
              <p className="truncate text-fg-muted" aria-live="polite">
                {statusLine}
              </p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
            <div className="flex items-center gap-2">
              <Select
                value={settings.interval}
                onValueChange={(value) =>
                  library.updateSettings({ interval: value as IntervalPreset })
                }
              >
                <SelectTrigger aria-label="How often the wallpaper changes" className="w-[172px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {INTERVALS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {settings.interval === "day" && (
                <Input
                  type="time"
                  aria-label="Time of the daily change"
                  value={settings.dailyTime}
                  className="w-[108px]"
                  onChange={(event) =>
                    event.target.value && library.updateSettings({ dailyTime: event.target.value })
                  }
                />
              )}
              {settings.interval === "custom" && (
                <CustomMinutes
                  value={settings.customMinutes}
                  onChange={(customMinutes) => library.updateSettings({ customMinutes })}
                />
              )}
            </div>
            <label className="flex items-center gap-2 text-sm">
              Shuffle
              <Switch
                checked={settings.shuffle}
                onCheckedChange={(shuffle) => library.updateSettings({ shuffle })}
              />
            </label>
          </div>
        </div>

        {/* ── Only exists once a second display is connected ── */}
        {multi && (
          <div className="mt-7">
            <div
              role="radiogroup"
              aria-label="How wallpapers fill your displays"
              className="inline-flex rounded-[10px] bg-surface p-1"
            >
              {MODES.map((mode) => {
                const selected = settings.displayMode === mode.value;
                return (
                  <button
                    key={mode.value}
                    type="button"
                    role="radio"
                    aria-checked={selected}
                    onClick={() => library.updateSettings({ displayMode: mode.value })}
                    className={cn(
                      "relative flex h-9 items-center gap-2 rounded-[7px] px-3 text-sm transition-colors duration-150",
                      selected ? "font-medium text-fg" : "text-fg-muted hover:text-fg",
                    )}
                  >
                    {/* The highlight slides to the chosen option. */}
                    {selected && (
                      <motion.span
                        layoutId="display-mode"
                        className="absolute inset-0 rounded-[7px] bg-raised shadow-[inset_0_0_0_1px_var(--color-line-strong)]"
                      />
                    )}
                    <ModeGlyph
                      mode={mode.value}
                      className={cn("relative", selected && "text-tint")}
                    />
                    <span className="relative">{mode.label}</span>
                  </button>
                );
              })}
            </div>
            <p className="mt-2 text-sm text-fg-muted">
              {MODES.find((mode) => mode.value === settings.displayMode)?.hint}
            </p>
          </div>
        )}

        {/* ── Which collections take part: a grid, so it scales to many ── */}
        <section className="mt-9" aria-labelledby="rotation-collections">
          <div className="mb-3 flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
            <h2 id="rotation-collections" className="text-lg font-semibold">
              Collections
            </h2>
            <FitSummary use={use} mode={displayMode} displayLabel={displayLabel} />
          </div>
          {state.collections.length === 0 ? (
            <p className="text-sm text-fg-muted">
              You have no collections yet.{" "}
              <button
                type="button"
                className="text-tint underline-offset-2 hover:underline"
                onClick={onOpenLibrary}
              >
                Open Library
              </button>
            </p>
          ) : (
            <div
              className="grid gap-2"
              style={{ gridTemplateColumns: "repeat(auto-fill, minmax(232px, 1fr))" }}
            >
              {state.collections.map((collection) => (
                <CollectionToggle
                  key={collection.id}
                  collection={collection}
                  wallpapers={library.wallpapersIn(collection.id)}
                  onChange={(value) => library.setInRotation(collection.id, value)}
                />
              ))}
            </div>
          )}
        </section>

        {/* ── Only exists once a live wallpaper is in play ── */}
        {hasLive && (
          <label className="mt-8 flex items-center justify-between gap-4 border-t border-line pt-4 text-sm">
            Pause live wallpapers on battery
            <Switch
              checked={settings.pauseLiveOnBattery}
              onCheckedChange={(pauseLiveOnBattery) =>
                library.updateSettings({ pauseLiveOnBattery })
              }
            />
          </label>
        )}

        {!canSetWallpaper && (
          <p className="mt-8 text-xs text-fg-faint">
            This is the browser preview. The Mac app is what changes your real desktop.
          </p>
        )}
      </div>

      <WallpaperPicker display={choosingFor} onClose={() => setChoosingFor(null)} />
    </div>
  );
}

/** One collection as a compact on/off card. The whole card is the switch. */
function CollectionToggle({
  collection,
  wallpapers,
  onChange,
}: {
  collection: Collection;
  wallpapers: Wallpaper[];
  onChange: (value: boolean) => void;
}) {
  const { urlOf } = useLibrary();
  const cover = wallpapers[0];
  const on = collection.inRotation;
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      onClick={() => onChange(!on)}
      className={cn(
        "flex items-center gap-3 rounded-[10px] p-2 pr-3 text-left transition-[background-color,box-shadow,transform] duration-150 active:scale-[0.985]",
        on
          ? "bg-raised shadow-[inset_0_0_0_1.5px_var(--tint)]"
          : "bg-surface shadow-[inset_0_0_0_1px_transparent] hover:bg-raised",
      )}
    >
      <Picture
        src={cover ? urlOf(cover.thumb) : undefined}
        alt=""
        color={cover?.color}
        className={cn(
          "h-10 w-16 shrink-0 rounded-[6px] transition-opacity duration-150",
          !on && "opacity-60",
        )}
      />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{collection.name}</span>
        <span className="block text-xs text-fg-muted">
          {plural(wallpapers.length, "wallpaper")}
        </span>
      </span>
      <span
        aria-hidden
        className={cn(
          "flex size-5 shrink-0 items-center justify-center rounded-full transition-colors duration-150",
          on ? "bg-tint text-tint-ink" : "shadow-[inset_0_0_0_1.5px_var(--color-line-strong)]",
        )}
      >
        <AnimatePresence initial={false}>
          {on && (
            <motion.span
              initial={{ scale: 0.4, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              exit={{ scale: 0.4, opacity: 0 }}
              transition={{ duration: 0.14 }}
            >
              <Check className="size-3.5" strokeWidth={3} />
            </motion.span>
          )}
        </AnimatePresence>
      </span>
    </button>
  );
}

/**
 * A few words on what happens to the chosen wallpapers on these displays.
 * Silent when everything is simply shown.
 */
function FitSummary({
  use,
  mode,
  displayLabel,
}: {
  use: { spanned: number; shown: number; skipped: number };
  mode: DisplayMode;
  displayLabel: string;
}) {
  const parts: string[] = [];
  if (mode === "span" && (use.shown > 0 || use.skipped > 0)) {
    if (use.spanned) parts.push(`${use.spanned} stretch across`);
    if (use.shown) parts.push(`${use.shown} shown on each display instead`);
  }
  if (use.skipped) parts.push(`${use.skipped} skipped as too small for ${displayLabel}`);
  if (parts.length === 0) return null;
  return <p className="text-sm text-fg-muted">{parts.join(", ")}</p>;
}

/** A number of minutes, checked when the person finishes typing rather than on every key. */
function CustomMinutes({
  value,
  onChange,
}: {
  value: number;
  onChange: (minutes: number) => void;
}) {
  const [draft, setDraft] = useState(String(value));
  const hintId = useId();
  useEffect(() => setDraft(String(value)), [value]);
  const commit = () => {
    const clamped = clampCustomMinutes(Number(draft));
    setDraft(String(clamped));
    if (clamped !== value) onChange(clamped);
  };
  return (
    <div className="flex items-center gap-1.5">
      <Input
        type="number"
        inputMode="numeric"
        min={MIN_CUSTOM_MINUTES}
        max={MAX_CUSTOM_MINUTES}
        value={draft}
        aria-label="Minutes between changes"
        aria-describedby={hintId}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={commit}
        onKeyDown={(event) => event.key === "Enter" && commit()}
        className="w-[72px] tabular-nums"
      />
      <span id={hintId} className="text-sm text-fg-muted">
        min
      </span>
    </div>
  );
}

/** Re-render periodically so "in 12 min" stays true while the page is open. */
function useClock(active: boolean): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    if (!active) return;
    setNow(new Date());
    const timer = setInterval(() => setNow(new Date()), 15_000);
    return () => clearInterval(timer);
  }, [active]);
  return now;
}
