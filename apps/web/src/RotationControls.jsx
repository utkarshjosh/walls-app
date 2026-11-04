import { nextChange } from "./schedule.js";

export function RotationControls({ state, update, onManageCollections, now }) {
  const next = nextChange(state, now);
  const selectedCount = state.collections.filter(c => c.enabled).length;
  const day = next?.toLocaleDateString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
  });
  const time = next?.toLocaleTimeString(undefined, {
    hour: "numeric",
    minute: "2-digit",
  });
  const minutes = next ? Math.max(1, Math.ceil((next - now) / 60000)) : 0;
  const until =
    minutes >= 60
      ? `${Math.floor(minutes / 60)}h${minutes % 60 ? ` ${minutes % 60}m` : ""}`
      : `${minutes}m`;
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return (
    <>
      <section className="next-change" aria-label="Next change preview">
        <span className="next-label">Next change · timing preview</span>
        <h3>
          {state.paused
            ? "Rotation is paused"
            : next
              ? time
              : "Add wallpapers to get started"}
        </h3>
        <p>
          {state.paused
            ? "Resume to see your next change."
            : next
              ? `${day} · in ${until}`
              : "Choose a collection with images on the Collections screen."}
        </p>
        <small>{zone.replaceAll("_", " ")} · your local time</small>
      </section>
      <div className="rotation-fields">
        <label className="field">
          Change wallpaper
          <select
            name="rotation-frequency"
            value={state.frequency}
            onChange={(e) =>
              update({ frequency: e.target.value })
            }
          >
            <option value="hour">Every hour</option>
            <option value="three">Every three hours</option>
            <option value="day">Every day</option>
          </select>
        </label>
        {state.frequency === "day" && (
          <label className="field">
            Daily at
            <input
              type="time"
              name="daily-change-time"
              value={state.dailyTime || "07:00"}
              onChange={(e) => {
                if (e.target.value) update({ dailyTime: e.target.value });
              }}
            />
          </label>
        )}
        <label className="field">
          Playback order
          <select
            name="rotation-order"
            value={state.mode}
            onChange={(e) => update({ mode: e.target.value })}
          >
            <option value="shuffle">Shuffle</option>
            <option value="order">In order</option>
          </select>
        </label>
      </div>
      <div className="rotation-source-summary">
        <span>{selectedCount === 0 ? "No collections selected" : `${selectedCount} collection${selectedCount === 1 ? "" : "s"} in rotation`}</span>
        <button className="text-button" onClick={onManageCollections}>Manage collections</button>
      </div>
      <p className="hint">
        Saved automatically in this browser. This previews your timing; desktop
        changes begin when the Mac app is connected.
      </p>
    </>
  );
}
