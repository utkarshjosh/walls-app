// Which wallpaper comes next.
//
// Pure rules shared in spirit with the Mac app's native core, which owns real
// rotation so it keeps running with the window closed. This copy powers the
// browser preview and documents the behaviour both must follow:
//
//  - A "frame" is what is on the desktop: one wallpaper id normally, or one per
//    display when each display shows something different.
//  - Shuffle draws from a bag holding every wallpaper once, so nothing repeats
//    until all have been shown. A fresh bag never starts with what is showing.
//  - In order walks the list and wraps.
//  - Previous steps back through what was actually shown, shuffled or not.

export interface EngineState {
  /** Ids still to be drawn in shuffle mode; the next one is at the front. */
  bag: string[];
  /** What is showing now. Empty until the app first sets a wallpaper. */
  current: string[];
  /** Earlier frames, most recent last. */
  history: string[][];
}

export const HISTORY_LIMIT = 50;

export function emptyEngine(): EngineState {
  return { bag: [], current: [], history: [] };
}

/** After the library changes: forget wallpapers that are gone, keep everything else. */
export function reconcile(state: EngineState, ids: readonly string[]): EngineState {
  const alive = new Set(ids);
  const keep = (frame: string[]) => frame.filter((id) => alive.has(id));
  return {
    bag: state.bag.filter((id) => alive.has(id)),
    current: keep(state.current),
    history: state.history.map(keep).filter((frame) => frame.length > 0),
  };
}

function shuffled(ids: readonly string[], random: () => number): string[] {
  const out = [...ids];
  for (let i = out.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1));
    [out[i], out[j]] = [out[j]!, out[i]!];
  }
  return out;
}

function pushHistory(state: EngineState): string[][] {
  if (state.current.length === 0) return state.history;
  return [...state.history, state.current].slice(-HISTORY_LIMIT);
}

/** Move to the next frame of `count` wallpapers. Returns the state unchanged when there is nothing to show. */
export function advance(
  state: EngineState,
  ids: readonly string[],
  options: { shuffle: boolean; count?: number; random?: () => number },
): EngineState {
  if (ids.length === 0) return state;
  const count = Math.max(1, options.count ?? 1);
  const random = options.random ?? Math.random;
  const frame: string[] = [];

  if (options.shuffle) {
    let bag = state.bag.filter((id) => ids.includes(id));
    const lastShown = state.current[state.current.length - 1];
    while (frame.length < count) {
      if (bag.length === 0) {
        bag = shuffled(ids, random);
        // Do not open a new round with the wallpaper that is on screen.
        const avoid = frame[frame.length - 1] ?? lastShown;
        if (bag.length > 1 && bag[0] === avoid) bag.push(bag.shift()!);
      }
      const next = bag.shift()!;
      // Different displays should not show the same picture when there is a choice.
      if (frame.includes(next) && ids.length >= count) {
        bag.push(next);
        continue;
      }
      frame.push(next);
    }
    return { bag, current: frame, history: pushHistory(state) };
  }

  const lastShown = state.current[state.current.length - 1];
  let index = lastShown === undefined ? -1 : ids.indexOf(lastShown);
  while (frame.length < count) {
    index = (index + 1) % ids.length;
    frame.push(ids[index]!);
  }
  return { bag: [], current: frame, history: pushHistory(state) };
}

/** Step back to the previously shown frame, or null when there is no history. */
export function back(state: EngineState): EngineState | null {
  if (state.history.length === 0) return null;
  const history = state.history.slice(0, -1);
  const current = state.history[state.history.length - 1]!;
  return { bag: state.bag, current, history };
}

/** The person picked a wallpaper by hand: show it and remember what it replaced. */
export function show(state: EngineState, frame: readonly string[]): EngineState {
  if (frame.length === 0) return state;
  return {
    bag: state.bag.filter((id) => !frame.includes(id)),
    current: [...frame],
    history: pushHistory(state),
  };
}
