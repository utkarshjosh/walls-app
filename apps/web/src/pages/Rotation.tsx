import { useEffect, useId, useState, type ReactNode } from "react";
import { Pause, Play, SkipBack, SkipForward } from "lucide-react";
import {
  MAX_CUSTOM_MINUTES,
  MIN_CUSTOM_MINUTES,
  clampCustomMinutes,
  describeWait,
  type IntervalPreset,
} from "@/lib/schedule";
import { canSpan, fitsDisplay, placementFor, rotationUse } from "@/lib/fit";
import { cn, plural } from "@/lib/utils";
import type { DisplayMode } from "@/platform/types";
import { useLibrary } from "@/state/store";
import { isLive, type Wallpaper } from "@/state/model";
import { DisplayPreview, ModeGlyph, orderedDisplays } from "@/components/DisplayPreview";
import { Picture } from "@/components/Picture";
import { Button } from "@/components/ui/button";
import {
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
} from "@/components/ui/form";
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
  { value: "span", label: "One across all", hint: "One wallpaper stretches across your displays." },
  { value: "each", label: "Different on each", hint: "Each display gets its own wallpaper." },
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
  const now = useClock(status.nextChangeAt !== null);

  const run = async (name: Parameters<typeof library.command>[0]) => {
    setWorking(true);
    try {
      await library.command(name);
    } finally {
      setWorking(false);
    }
  };

  const current = status.current
    .map((entry) => state.wallpapers[entry.itemId])
    .filter((wallpaper): wallpaper is Wallpaper => Boolean(wallpaper));
  const started = current.length > 0;

  // Only wallpapers large enough for these displays take part (src/lib/fit.ts).
  const use = rotationUse(rotationWallpapers, displays, displayMode);
  const usable = use.spanned + use.shown;
  const ordered = orderedDisplays(displays);
  // Before anything has been shown, preview what starting would put up first.
  const firstUp =
    displayMode === "each"
      ? ordered
          .map((display) => rotationWallpapers.find((wallpaper) => fitsDisplay(wallpaper, display)))
          .filter((wallpaper): wallpaper is Wallpaper => Boolean(wallpaper))
      : rotationWallpapers
          .filter((wallpaper) => placementFor(wallpaper, displays, displayMode) !== "skip")
          .slice(0, 1);
  const previewed = started ? current : firstUp;
  // In "one across all", a picture too small to stretch is shown on each display instead.
  const previewMode =
    displayMode === "span" && previewed[0] && !canSpan(previewed[0], displays)
      ? "same"
      : displayMode;
  const pictures = previewed.map((wallpaper) => ({
    src: urlOf(
      wallpaper.kind === "image" ? wallpaper.full : (wallpaper.poster ?? wallpaper.thumb),
    ),
    color: wallpaper.color,
  }));

  const noneChosen = rotationWallpapers.length === 0;
  // Nothing to rotate: either no collection is on, or every wallpaper is too small.
  const empty = usable === 0;
  const displayLabel =
    displays.length > 1 ? "your displays" : (displays[0]?.name ?? "this display");
  const nextAt = status.nextChangeAt ? new Date(status.nextChangeAt) : null;
  const hasLive = rotationWallpapers.some(isLive) || current.some(isLive);

  return (
    <div className="scroll-area h-full">
      <div className="mx-auto max-w-[1120px] px-8 pt-7 pb-16">
        <PageHeader title="Rotation" />

        <div className="grid gap-x-12 gap-y-10 lg:grid-cols-[minmax(0,1fr)_minmax(0,400px)]">
          {/* ── What is on the desktop, and the three transport controls ── */}
          <section aria-label="Now showing" className="lg:sticky lg:top-7 lg:self-start">
            <div className="relative flex min-h-[220px] items-center justify-center rounded-panel bg-surface px-8 py-10">
              {displays.length > 0 && pictures.length > 0 ? (
                <DisplayPreview
                  displays={displays}
                  mode={previewMode}
                  pictures={pictures}
                  className={cn(
                    "max-w-[560px] transition-opacity duration-200",
                    !started && "opacity-40",
                  )}
                />
              ) : noneChosen ? (
                <p className="max-w-xs text-center text-sm text-fg-muted">
                  Turn on a collection and its wallpapers will take turns on your desktop.
                </p>
              ) : (
                <div className="max-w-sm text-center">
                  <p className="font-medium">Nothing here is large enough for {displayLabel}</p>
                  <p className="mt-1 text-sm text-fg-muted">
                    These wallpapers would look blurry, so they are not used. Larger ones are in
                    Discover.
                  </p>
                  <Button className="mt-4" onClick={onOpenDiscover}>
                    Open Discover
                  </Button>
                </div>
              )}
              {!started && !empty && (
                <div className="absolute inset-0 flex items-center justify-center">
                  {/* Nothing has been put on the desktop yet. If rotation is
                      already running, the first change would otherwise wait
                      for the next scheduled time. */}
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
            </div>

            <div className="mt-5 flex items-center gap-5">
              <div className="flex items-center gap-1.5">
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
                {started && (
                  <p className="truncate font-medium">
                    {[...new Set(current.map((wallpaper) => wallpaper.title))].join(" and ")}
                  </p>
                )}
                <p className="text-fg-muted" aria-live="polite">
                  {noneChosen
                    ? "No collections are turned on."
                    : empty
                      ? "No wallpaper is large enough to show."
                      : settings.paused
                      ? started
                        ? "Paused. This wallpaper stays until you resume."
                        : "Rotation is off."
                      : nextAt
                        ? `Next change at ${nextAt.toLocaleTimeString(undefined, {
                            hour: "numeric",
                            minute: "2-digit",
                          })}, ${describeWait(nextAt, now)}`
                        : ""}
                </p>
              </div>
            </div>

            {!canSetWallpaper && (
              <p className="mt-6 text-xs text-fg-faint">
                You're looking at a preview in the browser. The Mac app is what changes your real
                desktop.
              </p>
            )}
          </section>

          {/* ── The few choices that shape rotation ── */}
          <div className="space-y-9">
            <Group title="Collections" note="Wallpapers come from the collections you turn on.">
              {state.collections.length === 0 ? (
                <p className="text-sm text-fg-muted">
                  You have no collections yet.{" "}
                  <button type="button" className="text-tint underline-offset-2 hover:underline" onClick={onOpenLibrary}>
                    Open Library
                  </button>
                </p>
              ) : (
                <ul className="-mx-2">
                  {state.collections.map((collection) => {
                    const wallpapers = library.wallpapersIn(collection.id);
                    const cover = wallpapers[0];
                    const id = `rotation-${collection.id}`;
                    return (
                      <li key={collection.id}>
                        <label
                          htmlFor={id}
                          className="flex items-center gap-3 rounded-control px-2 py-1.5 hover:bg-surface"
                        >
                          <Picture
                            src={cover ? urlOf(cover.thumb) : undefined}
                            alt=""
                            color={cover?.color}
                            className="h-9 w-14 shrink-0 rounded-[5px]"
                          />
                          <span className="min-w-0 flex-1">
                            <span className="block truncate font-medium">{collection.name}</span>
                            <span className="block text-xs text-fg-muted">
                              {plural(wallpapers.length, "wallpaper")}
                            </span>
                          </span>
                          <Switch
                            id={id}
                            checked={collection.inRotation}
                            onCheckedChange={(value) => library.setInRotation(collection.id, value)}
                          />
                        </label>
                      </li>
                    );
                  })}
                </ul>
              )}
              <FitSummary use={use} mode={displayMode} displayLabel={displayLabel} />
            </Group>

            <Group title="Timing">
              <Row label="Change wallpaper" htmlFor="rotation-interval">
                <Select
                  value={settings.interval}
                  onValueChange={(value) =>
                    library.updateSettings({ interval: value as IntervalPreset })
                  }
                >
                  <SelectTrigger id="rotation-interval" className="w-48">
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
              </Row>
              {settings.interval === "day" && (
                <Row label="At" htmlFor="rotation-time">
                  <Input
                    id="rotation-time"
                    type="time"
                    value={settings.dailyTime}
                    className="w-48"
                    onChange={(event) =>
                      event.target.value && library.updateSettings({ dailyTime: event.target.value })
                    }
                  />
                </Row>
              )}
              {settings.interval === "custom" && (
                <Row label="Every" htmlFor="rotation-custom">
                  <CustomMinutes
                    id="rotation-custom"
                    value={settings.customMinutes}
                    onChange={(customMinutes) => library.updateSettings({ customMinutes })}
                  />
                </Row>
              )}
              <Row label="Shuffle" htmlFor="rotation-shuffle">
                <Switch
                  id="rotation-shuffle"
                  checked={settings.shuffle}
                  onCheckedChange={(shuffle) => library.updateSettings({ shuffle })}
                />
              </Row>
            </Group>

            {/* Only exists once a second display is connected. */}
            {displays.length > 1 && (
              <Group
                title="Displays"
                note={`${orderedDisplays(displays)
                  .map((display) => display.name)
                  .join(", ")}`}
              >
                <div role="radiogroup" aria-label="How wallpapers fill your displays" className="grid grid-cols-3 gap-2">
                  {MODES.map((mode) => {
                    const selected = settings.displayMode === mode.value;
                    return (
                      <button
                        key={mode.value}
                        type="button"
                        role="radio"
                        aria-checked={selected}
                        title={mode.hint}
                        onClick={() => library.updateSettings({ displayMode: mode.value })}
                        className={cn(
                          "flex flex-col items-center gap-2 rounded-control px-2 pt-3.5 pb-2.5 text-sm transition-colors duration-100",
                          selected
                            ? "bg-raised font-medium text-fg shadow-[inset_0_0_0_1.5px_var(--tint)]"
                            : "bg-surface text-fg-muted hover:bg-raised hover:text-fg",
                        )}
                      >
                        <ModeGlyph mode={mode.value} className={selected ? "text-tint" : undefined} />
                        {mode.label}
                      </button>
                    );
                  })}
                </div>
                <p className="mt-2.5 text-sm text-fg-muted">
                  {MODES.find((mode) => mode.value === settings.displayMode)?.hint}
                </p>
              </Group>
            )}

            {/* Only exists once a live wallpaper is in play. */}
            {hasLive && (
              <Group title="Live wallpapers">
                <Row label="Pause on battery" htmlFor="rotation-battery">
                  <Switch
                    id="rotation-battery"
                    checked={settings.pauseLiveOnBattery}
                    onCheckedChange={(pauseLiveOnBattery) =>
                      library.updateSettings({ pauseLiveOnBattery })
                    }
                  />
                </Row>
              </Group>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

/**
 * One line that says what happens to the chosen wallpapers on these displays.
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
  if (mode === "span") {
    if (use.spanned) parts.push(`${use.spanned} stretch across your displays`);
    if (use.shown)
      parts.push(
        `${use.shown} ${use.shown === 1 ? "is" : "are"} too small to stretch and ${use.shown === 1 ? "appears" : "appear"} on each display`,
      );
  }
  if (use.skipped)
    parts.push(
      `${use.skipped} ${use.skipped === 1 ? "is" : "are"} too small for ${displayLabel} and ${use.skipped === 1 ? "is" : "are"} skipped`,
    );
  if (parts.length === 0 || (mode === "span" && !use.shown && !use.skipped)) return null;
  const sentence = parts.join("; ");
  return (
    <p className="mt-3 text-sm text-fg-muted">
      {sentence.charAt(0).toUpperCase() + sentence.slice(1)}.
    </p>
  );
}

function Group({ title, note, children }: { title: string; note?: string; children: ReactNode }) {
  return (
    <section>
      <h2 className="text-lg font-semibold">{title}</h2>
      {note && <p className="mt-0.5 text-sm text-fg-muted">{note}</p>}
      <div className="mt-3">{children}</div>
    </section>
  );
}

function Row({ label, htmlFor, children }: { label: string; htmlFor: string; children: ReactNode }) {
  return (
    <div className="flex min-h-11 items-center justify-between gap-4 border-b border-line/70 last:border-0">
      <Label htmlFor={htmlFor} className="text-base text-fg">
        {label}
      </Label>
      {children}
    </div>
  );
}

/** A number of minutes, checked when the person finishes typing rather than on every key. */
function CustomMinutes({
  id,
  value,
  onChange,
}: {
  id: string;
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
    <div className="flex w-48 items-center gap-2">
      <Input
        id={id}
        type="number"
        inputMode="numeric"
        min={MIN_CUSTOM_MINUTES}
        max={MAX_CUSTOM_MINUTES}
        value={draft}
        aria-describedby={hintId}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={commit}
        onKeyDown={(event) => event.key === "Enter" && commit()}
        className="tabular-nums"
      />
      <span id={hintId} className="shrink-0 text-sm text-fg-muted">
        minutes
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
