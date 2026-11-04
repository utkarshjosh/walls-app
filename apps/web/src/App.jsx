import { useState, useEffect, useRef } from "react";
import {
  MotionConfig,
  motion,
  AnimatePresence,
  useReducedMotion,
} from "motion/react";
import * as Dialog from "@radix-ui/react-dialog";
import {
  GearIcon,
  ArrowRightIcon,
  Cross2Icon,
  ArrowLeftIcon,
  DownloadIcon,
} from "@radix-ui/react-icons";
import { openDB } from "idb";
import { RotationControls } from "./RotationControls.jsx";
import "@fontsource-variable/inter";
const seed = [
  {
    id: "aviation",
    name: "Aviation",
    enabled: true,
    images: [
      ["glider", "Above the clouds"],
      ["wing", "Golden hour"],
      ["coastal-flight", "Coastal flight"],
    ],
  },
  {
    id: "landscapes",
    name: "Landscapes",
    enabled: true,
    images: [
      ["alpine", "Alpine light"],
      ["coast", "Black sand coast"],
      ["ridges", "Morning ridges"],
    ],
  },
  {
    id: "abstract",
    name: "Abstract",
    enabled: false,
    images: [
      ["paper", "Paper forms"],
      ["limestone", "Quiet stone"],
      ["shadows", "Light and shadow"],
    ],
  },
].map((c) => ({
  ...c,
  images: c.images.map(([id, title]) => ({
    id,
    title,
    src: `/wallpapers/${id}.jpg`,
  })),
}));
const database = () =>
  openDB("walls-local", 1, {
    upgrade(db) {
      db.createObjectStore("state");
    },
  });
function Modal({ open, onClose, title, description, children, wide }) {
  const reduce = useReducedMotion();
  const returnFocus = useRef(null);
  return (
    <Dialog.Root open={open} onOpenChange={(v) => !v && onClose()}>
      <AnimatePresence>
        {open && (
          <Dialog.Portal forceMount>
            <Dialog.Overlay asChild>
              <motion.div
                className="overlay"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
              />
            </Dialog.Overlay>
            <Dialog.Content
              asChild
              onOpenAutoFocus={() => {
                returnFocus.current = document.activeElement;
              }}
              onCloseAutoFocus={(event) => {
                event.preventDefault();
                returnFocus.current?.focus();
              }}
            >
              <motion.section
                className={`dialog ${wide ? "wide" : ""}`}
                initial={{ opacity: 0, y: reduce ? 0 : 6 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0 }}
              >
                <div className="dialog-heading">
                  <Dialog.Title>{title}</Dialog.Title>
                  <Dialog.Close className="icon-button" aria-label="Close">
                    <Cross2Icon />
                  </Dialog.Close>
                </div>
                <Dialog.Description>{description}</Dialog.Description>
                {children}
              </motion.section>
            </Dialog.Content>
          </Dialog.Portal>
        )}
      </AnimatePresence>
    </Dialog.Root>
  );
}
export function App() {
  const [state, setState] = useState({
      collections: seed,
      mode: "shuffle",
      frequency: "hour",
      dailyTime: "07:00",
      rotationAnchor: Date.now(),
      paused: false,
    }),
    [loaded, setLoaded] = useState(false),
    [storageError, setStorageError] = useState("");
  const [tab, setTab] = useState("collections"),
    [collection, setCollection] = useState(null),
    [modal, setModal] = useState(null),
    [preview, setPreview] = useState(null),
    [notice, setNotice] = useState(""),
    [name, setName] = useState(""),
    [destination, setDestination] = useState("aviation"),
    [files, setFiles] = useState([]),
    [results, setResults] = useState([]),
    [busy, setBusy] = useState(false),
    [query, setQuery] = useState("");
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    if (modal !== "settings" && modal !== "rotation") return;
    setNow(new Date());
    const timer = setInterval(() => setNow(new Date()), 15000);
    return () => clearInterval(timer);
  }, [modal]);
  useEffect(() => {
    let alive = true;
    database()
      .then((db) => db.get("state", "library"))
      .then((saved) => {
        if (alive) {
          if (saved?.collections)
            setState((current) => ({ ...current, ...saved }));
          setLoaded(true);
        }
      })
      .catch(() => {
        if (alive) {
          setLoaded(true);
          setStorageError(
            "Local saving is unavailable. Changes will last for this session.",
          );
        }
      });
    return () => {
      alive = false;
    };
  }, []);
  useEffect(() => {
    if (loaded)
      database()
        .then((db) => db.put("state", state, "library"))
        .catch(() =>
          setStorageError(
            "Could not save locally. Your browser storage may be full.",
          ),
        );
  }, [state, loaded]);
  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(""), 3500);
    return () => clearTimeout(t);
  }, [notice]);
  const update = (patch) => { setNow(new Date()); setState((s) => ({ ...s, ...patch })); };
  const toggle = (id) =>
    setState((s) => ({
      ...s,
      collections: s.collections.map((c) =>
        c.id === id ? { ...c, enabled: !c.enabled } : c,
      ),
    }));
  const selected = state.collections
    .filter((c) => c.enabled)
    .map((c) => c.name);
  const names =
    selected.length < 3
      ? selected.join(" and ")
      : `${selected.slice(0, -1).join(", ")} and ${selected.at(-1)}`;
  const cadence = {
    hour: "every hour",
    three: "every three hours",
    day: `every day at ${state.dailyTime || "07:00"}`,
  }[state.frequency];
  const summary = selected.length
    ? `${names}, ${state.mode === "shuffle" ? "shuffled" : "in order"} ${cadence}.`
    : "Choose a collection to start your wallpaper rotation.";
  const current = state.collections.find((c) => c.id === collection);
  const all = Array.from(
    new Map(
      state.collections.flatMap((c) => c.images).map((i) => [i.id, i]),
    ).values(),
  );
  const images = (current ? current.images : all).filter((i) =>
    i.title.toLowerCase().includes(query.toLowerCase()),
  );
  function openUpload() {
    setDestination(current?.id || state.collections[0]?.id || "");
    setFiles([]);
    setResults([]);
    setModal("upload");
  }
  function create(e) {
    e.preventDefault();
    const title = name.trim();
    if (!title) return;
    if (
      state.collections.some(
        (c) => c.name.toLowerCase() === title.toLowerCase(),
      )
    ) {
      setNotice("A collection with that name already exists.");
      return;
    }
    const id = crypto.randomUUID();
    setState((s) => ({
      ...s,
      collections: [
        ...s.collections,
        { id, name: title, enabled: false, images: [] },
      ],
    }));
    setModal(null);
    setName("");
    setCollection(id);
    setTab("collections");
    setNotice("Collection created");
  }
  async function upload(e) {
    e.preventDefault();
    if (!destination || !files.length) return;
    setBusy(true);
    const outcomes = [],
      additions = [];
    for (const file of files) {
      try {
        if (!["image/jpeg", "image/png"].includes(file.type))
          throw Error("Choose a JPEG or PNG image.");
        if (file.size > 25 * 1024 * 1024)
          throw Error("This file exceeds 25 MB.");
        const src = await new Promise((resolve, reject) => {
          const reader = new FileReader();
          reader.onload = () => resolve(reader.result);
          reader.onerror = () => reject(Error("Could not read this file."));
          reader.readAsDataURL(file);
        });
        const image = await new Promise((resolve, reject) => {
          const img = new Image();
          img.onload = () => resolve(img);
          img.onerror = () => reject(Error("This image could not be opened."));
          img.src = src;
        });
        if (image.naturalWidth * image.naturalHeight > 60000000)
          throw Error("This image exceeds 60 megapixels.");
        additions.push({
          id: crypto.randomUUID(),
          title: file.name.replace(/\.[^.]+$/, ""),
          src,
          uploaded: true,
        });
        outcomes.push({ name: file.name, ok: true, message: "Added" });
      } catch (error) {
        outcomes.push({ name: file.name, ok: false, message: error.message });
      }
    }
    setState((s) => ({
      ...s,
      collections: s.collections.map((c) =>
        c.id === destination
          ? { ...c, images: [...c.images, ...additions] }
          : c,
      ),
    }));
    setResults(outcomes);
    setFiles([]);
    setBusy(false);
    if (additions.length)
      setNotice(
        `${additions.length} wallpaper${additions.length === 1 ? "" : "s"} added locally`,
      );
  }
  const gallery = (list) => (
    <div className="image-grid">
      {list.map((image) => (
        <button
          className="image-button"
          key={image.id}
          onClick={() => setPreview(image)}
          aria-label={`Preview ${image.title}`}
        >
          <img src={image.src} alt={image.title} />
        </button>
      ))}
    </div>
  );
  return (
    <MotionConfig reducedMotion="user" transition={{ duration: 0.18 }}>
      <header className="topbar">
        <button
          className="brand"
          onClick={() => {
            setTab("collections");
            setCollection(null);
          }}
        >
          Walls
        </button>
        <nav aria-label="Main navigation">
          <button
            className={tab === "collections" ? "active" : ""}
            onClick={() => {
              setTab("collections");
              setCollection(null);
              setQuery("");
            }}
          >
            Collections
          </button>
          <button
            className={tab === "all" ? "active" : ""}
            onClick={() => {
              setTab("all");
              setCollection(null);
              setQuery("");
            }}
          >
            All wallpapers
          </button>
        </nav>
        <button className="settings" onClick={() => setModal("settings")}>
          <GearIcon />
          <span>Settings</span>
        </button>
      </header>
      <main>
        <section className="page-heading">
          <div>
            {current && (
              <button
                className="back text-button"
                onClick={() => setCollection(null)}
              >
                <ArrowLeftIcon />
                Collections
              </button>
            )}
            <h1>
              {current
                ? current.name
                : tab === "all"
                  ? "All wallpapers"
                  : "Your collections"}
            </h1>
            <p className="summary">
              {state.paused ? "Rotation paused. " : ""}
              {summary}
              <button className="edit" onClick={() => setModal("rotation")}>
                Edit
              </button>
            </p>
          </div>
          <div className="actions">
            <button className="primary" onClick={openUpload}>
              Add wallpapers
            </button>
            <button
              className="text-button"
              onClick={() => {
                setName("");
                setModal("new");
              }}
            >
              New collection
            </button>
          </div>
        </section>
        {storageError && (
          <p className="warning" role="status">
            {storageError}
          </p>
        )}
        {!loaded ? (
          <div className="skeleton" aria-label="Loading collections" />
        ) : tab === "collections" && !current ? (
          <div>
            {state.collections.map((c) => (
              <section className="shelf" key={c.id}>
                <div className="shelf-heading">
                  <div>
                    <h2>{c.name}</h2>
                    <button
                      className="open text-button"
                      onClick={() => {
                        setCollection(c.id);
                        setQuery("");
                      }}
                    >
                      Open
                      <ArrowRightIcon />
                    </button>
                  </div>
                  <label className="check">
                    <input
                      name="rotation-collection"
                      type="checkbox"
                      checked={c.enabled}
                      onChange={() => toggle(c.id)}
                    />
                    Use for wallpapers
                  </label>
                </div>
                {c.images.length ? (
                  gallery(c.images.slice(0, 3))
                ) : (
                  <div className="empty">
                    <p>A little space for your next find.</p>
                    <button
                      className="text-button"
                      onClick={() => {
                        setCollection(c.id);
                        setDestination(c.id);
                        setFiles([]);
                        setResults([]);
                        setModal("upload");
                      }}
                    >
                      Add wallpapers to {c.name}
                    </button>
                  </div>
                )}
              </section>
            ))}
          </div>
        ) : (
          <>
            <div className="library-toolbar">
              {current ? (
                <label className="check">
                  <input
                    name="rotation-collection"
                    type="checkbox"
                    checked={current.enabled}
                    onChange={() => toggle(current.id)}
                  />
                  Use for wallpapers
                </label>
              ) : (
                <p>{all.length} wallpapers</p>
              )}
              <input
                className="search"
                name="search"
                type="search"
                placeholder="Find a wallpaper"
                aria-label="Find a wallpaper"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
            {images.length ? (
              gallery(images)
            ) : (
              <div className="empty">
                <h2>
                  {query
                    ? "No matching wallpapers"
                    : "Make this collection yours"}
                </h2>
                <p>
                  {query
                    ? "Try another name."
                    : "Add a few images you love to get started."}
                </p>
                {!query && (
                  <button className="primary" onClick={openUpload}>
                    Add wallpapers
                  </button>
                )}
              </div>
            )}
          </>
        )}
        <footer>
          <span>Sample collection imagery</span>
          <span>Local preview · changes stay in this browser</span>
        </footer>
      </main>
      <Modal
        open={modal === "new"}
        onClose={() => setModal(null)}
        title="New collection"
        description="A home for wallpapers you love."
      >
        <form onSubmit={create}>
          <label className="field">
            Collection name
            <input
              name="collection-name"
              autoFocus
              maxLength={60}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Quiet mornings"
              required
            />
          </label>
          <div className="dialog-actions">
            <button
              type="button"
              className="secondary"
              onClick={() => setModal(null)}
            >
              Cancel
            </button>
            <button className="primary" disabled={!name.trim()}>
              Create collection
            </button>
          </div>
        </form>
      </Modal>
      <Modal
        open={modal === "rotation"}
        onClose={() => setModal(null)}
        title="Your wallpaper rhythm"
        description="Choose your folders. We’ll take it from here."
      >
        <RotationControls
          state={state}
          update={update}
          toggle={toggle}
          now={now}
        />
        <div className="dialog-actions">
          <button className="primary" onClick={() => setModal(null)}>
            Done
          </button>
        </div>
      </Modal>
      <Modal
        open={modal === "upload"}
        onClose={() => !busy && setModal(null)}
        title="Add wallpapers"
        description="Pick a few images that make your day better."
      >
        <form onSubmit={upload}>
          <label className="field">
            Save to collection
            <select
              name="wallpaper-preference"
              value={destination}
              onChange={(e) => setDestination(e.target.value)}
            >
              {state.collections.map((c) => (
                <option value={c.id} key={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>
          <label className="file-picker">
            Choose JPEG or PNG images
            <input
              name="wallpapers"
              type="file"
              accept="image/jpeg,image/png"
              multiple
              onChange={(e) => {
                setFiles(Array.from(e.target.files));
                setResults([]);
              }}
              disabled={busy}
            />
            <span>Up to 25 MB and 60 megapixels per image</span>
          </label>
          {files.length > 0 && (
            <p>
              {files.length} file{files.length === 1 ? "" : "s"} selected
            </p>
          )}
          <ul className="upload-results">
            {results.map((r, i) => (
              <li key={i} className={r.ok ? "success" : "error"}>
                <strong>{r.name}</strong>
                <span>{r.message}</span>
              </li>
            ))}
          </ul>
          <p className="hint">
            Uploads stay in this browser during the preview.
          </p>
          <div className="dialog-actions">
            <button
              type="button"
              className="secondary"
              disabled={busy}
              onClick={() => setModal(null)}
            >
              Close
            </button>
            <button
              className="primary"
              disabled={!files.length || busy || !destination}
            >
              {busy ? "Adding images…" : "Add to collection"}
            </button>
          </div>
        </form>
      </Modal>
      <Modal
        open={modal === "settings"}
        onClose={() => setModal(null)}
        title="Settings"
        description="Your collections, on your time."
      >
        <RotationControls
          state={state}
          update={update}
          toggle={toggle}
          now={now}
        />
        <div className="setting-row">
          <div>
            <h3>Pause rotation</h3>
            <p>Keep your current wallpaper for a while.</p>
          </div>
          <input
            aria-label="Pause rotation"
            type="checkbox"
            checked={state.paused}
            onChange={(e) => update({ paused: e.target.checked })}
          />
        </div>
        <button
          className="secondary"
          onClick={() => {
            const a = document.createElement("a");
            a.href = URL.createObjectURL(
              new Blob([JSON.stringify(state, null, 2)], {
                type: "application/json",
              }),
            );
            a.download = "walls-collection-backup.json";
            a.click();
            setTimeout(() => URL.revokeObjectURL(a.href), 1000);
          }}
        >
          Export local collection backup
        </button>
      </Modal>
      <Modal
        open={Boolean(preview)}
        onClose={() => setPreview(null)}
        title={preview?.title || "Wallpaper"}
        description="A moment worth keeping."
        wide
      >
        {preview && (
          <>
            <img
              className="preview-image"
              src={preview.src}
              alt={preview.title}
            />
            <div className="dialog-actions">
              <a
                className="primary"
                href={preview.src}
                download={`${preview.title}.jpg`}
              >
                <DownloadIcon />
                Download image
              </a>
            </div>
          </>
        )}
      </Modal>
      <AnimatePresence>
        {notice && (
          <motion.div
            className="toast"
            role="status"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
          >
            {notice}
          </motion.div>
        )}
      </AnimatePresence>
    </MotionConfig>
  );
}
