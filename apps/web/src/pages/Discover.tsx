import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion } from "motion/react";
import { Bookmark, BookmarkCheck, Check, FolderPlus, Monitor, Search } from "lucide-react";
import {
  CatalogError,
  catalog,
  fileUrl,
  isPerDisplay,
  type CatalogCategory,
  type CatalogItem,
  type CatalogPack,
  type CatalogQuery,
} from "@/api/client";
import { canSpan, fitsAny } from "@/lib/fit";
import { fitNote } from "@/lib/fitNote";
import { cn, plural } from "@/lib/utils";
import { useLibrary } from "@/state/store";
import { WithContextMenu, type MenuEntry } from "@/components/ActionMenu";
import { Lightbox, type LightboxItem } from "@/components/Lightbox";
import { Picture } from "@/components/Picture";
import { SetWallpaperButton } from "@/components/SetWallpaperButton";
import { useToast } from "@/components/Toast";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/form";
import { useSetWallpaper } from "@/components/useSetWallpaper";
import { GridSkeleton, WallpaperGrid, WallpaperTile } from "@/components/WallpaperGrid";
import { EmptyState, PageHeader } from "./Library";

type Filter = { id: string; label: string; query: CatalogQuery };

/** The bookmark pops when a wallpaper becomes saved, so the click is felt. */
function SaveMark({ saved }: { saved: boolean }) {
  return (
    <motion.span
      key={String(saved)}
      initial={{ scale: saved ? 0.5 : 1 }}
      animate={{ scale: 1 }}
      transition={{ duration: 0.18 }}
      className="inline-flex"
    >
      {saved ? <BookmarkCheck /> : <Bookmark />}
    </motion.span>
  );
}

const itemAspect = (item: CatalogItem) =>
  item.width > 0 && item.height > 0 ? item.width / item.height : 16 / 10;

export function DiscoverPage({ onOpenCollection }: { onOpenCollection: (id: string) => void }) {
  const library = useLibrary();
  const { displays, savedRemoteIds, busyRemoteIds, state } = library;
  const notify = useToast();
  const { set: setWallpaper, busy: setting } = useSetWallpaper();

  const [categories, setCategories] = useState<CatalogCategory[]>([]);
  const [packs, setPacks] = useState<CatalogPack[]>([]);
  const [filterId, setFilterId] = useState("all");
  const [search, setSearch] = useState("");
  const [term, setTerm] = useState("");
  const [items, setItems] = useState<CatalogItem[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [phase, setPhase] = useState<"loading" | "ready" | "more" | "error">("loading");
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [viewing, setViewing] = useState<number | null>(null);
  const [savingPack, setSavingPack] = useState<string | null>(null);

  // A second display unlocks the panoramas filter; with one display it does not exist.
  const filters = useMemo<Filter[]>(
    () => [
      { id: "all", label: "All", query: {} },
      { id: "live", label: "Live", query: { kind: "live" } },
      ...(displays.length > 1
        ? [{ id: "wide", label: `For ${displays.length} displays`, query: { wide: true } }]
        : []),
      ...categories
        .filter((category) => category.slug !== "live")
        .map((category) => ({
          id: `c:${category.slug}`,
          label: category.name,
          query: { category: category.slug },
        })),
    ],
    [categories, displays.length],
  );
  const filter = filters.find((item) => item.id === filterId) ?? filters[0]!;

  useEffect(() => {
    const timer = setTimeout(() => setTerm(search.trim()), 250);
    return () => clearTimeout(timer);
  }, [search]);

  // Shelves and filter names. Failing here is fine: the grid below reports problems.
  useEffect(() => {
    const controller = new AbortController();
    catalog.categories(controller.signal).then(setCategories).catch(() => {});
    catalog.packs(controller.signal).then(setPacks).catch(() => {});
    return () => controller.abort();
  }, [attempt]);

  // First page for the current filter and search.
  useEffect(() => {
    const controller = new AbortController();
    setPhase("loading");
    setViewing(null);
    catalog
      .wallpapers({ ...filter.query, q: term || undefined }, controller.signal)
      .then((page) => {
        setItems(page.items);
        setCursor(page.nextCursor);
        setPhase("ready");
      })
      .catch((cause: unknown) => {
        if ((cause as Error).name === "AbortError") return;
        setItems([]);
        setCursor(null);
        setError(cause instanceof CatalogError ? cause.message : "Discover can't be reached right now.");
        setPhase("error");
      });
    return () => controller.abort();
    // filter.query is derived from filter.id.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filter.id, term, attempt]);

  const loadMore = useCallback(() => {
    if (phase !== "ready" || !cursor) return;
    setPhase("more");
    catalog
      .wallpapers({ ...filter.query, q: term || undefined, cursor })
      .then((page) => {
        setItems((current) => {
          const seen = new Set(current.map((item) => item.id));
          return [...current, ...page.items.filter((item) => !seen.has(item.id))];
        });
        setCursor(page.nextCursor);
        setPhase("ready");
      })
      .catch(() => setPhase("ready"));
  }, [phase, cursor, filter.query, term]);

  // Load the next page when the end of the grid scrolls into view.
  const sentinel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const element = sentinel.current;
    if (!element) return;
    const observer = new IntersectionObserver(
      (entries) => entries.some((entry) => entry.isIntersecting) && loadMore(),
      { rootMargin: "600px" },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [loadMore]);

  const toggleSave = async (item: CatalogItem) => {
    try {
      if (savedRemoteIds.has(item.id)) await library.unsaveItem(item.id);
      else await library.saveItem(item);
    } catch {
      notify(`“${item.title}” could not be saved. Check your connection and try again.`, {
        tone: "error",
      });
    }
  };

  const savePack = async (pack: CatalogPack) => {
    setSavingPack(pack.id);
    try {
      const full = await catalog.pack(pack.id);
      const collectionId = await library.savePack(pack, full.items);
      notify(`“${pack.name}” is in your library`, {
        action: { label: "Open", run: () => onOpenCollection(collectionId) },
      });
    } catch {
      notify(`“${pack.name}” could not be saved. Check your connection and try again.`, {
        tone: "error",
      });
    } finally {
      setSavingPack(null);
    }
  };

  const itemMenu = (item: CatalogItem): MenuEntry[] => [
    {
      type: "item",
      label: "Set as wallpaper",
      icon: Monitor,
      onSelect: () => setWallpaper(() => library.saveItem(item)),
    },
    {
      type: "sub",
      label: "Save to",
      icon: FolderPlus,
      entries: state.collections.map(
        (collection): MenuEntry => ({
          type: "item",
          label: collection.name,
          onSelect: () => {
            library
              .saveItem(item, collection.id)
              .catch(() => notify("That could not be saved. Try again.", { tone: "error" }));
          },
        }),
      ),
    },
  ];

  const viewerItems: LightboxItem[] = items.map((item) => ({
    key: item.id,
    title: item.title,
    kind: item.kind,
    image: fileUrl(item.kind === "image" ? item.files.preview : (item.files.poster ?? item.files.preview)),
    live: item.kind === "image" ? undefined : fileUrl(item.files.original),
    color: item.color,
    aspect: itemAspect(item),
    detail:
      item.kind === "scene"
        ? `Live scene by ${item.author}`
        : `${item.width} × ${item.height}, by ${item.author}`,
    warning: fitNote(item, displays),
  }));

  const browsing = filter.id === "all" && !term;

  return (
    <div className="scroll-area h-full">
      <div className="mx-auto max-w-[1320px] px-8 pt-7 pb-16">
        <PageHeader title="Discover">
          <div className="relative">
            <Search className="pointer-events-none absolute top-2 left-2.5 size-4 text-fg-faint" />
            <Input
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search wallpapers"
              aria-label="Search wallpapers"
              className="w-60 pl-8"
            />
          </div>
        </PageHeader>

        <div
          className="no-scrollbar -mx-1 mb-6 flex gap-1.5 overflow-x-auto px-1 py-1"
          role="group"
          aria-label="Filter wallpapers"
        >
          {filters.map((item) => (
            <button
              key={item.id}
              type="button"
              aria-pressed={item.id === filter.id}
              onClick={() => setFilterId(item.id)}
              className={cn(
                "h-7 shrink-0 rounded-full px-3 text-sm transition-colors duration-100",
                item.id === filter.id
                  ? "bg-fg font-medium text-bg"
                  : "bg-raised text-fg-muted hover:bg-hover hover:text-fg",
              )}
            >
              {item.label}
            </button>
          ))}
        </div>

        {browsing && packs.length > 0 && phase !== "error" && (
          <section className="mb-10" aria-labelledby="discover-collections">
            <h2 id="discover-collections" className="mb-3 text-lg font-semibold">
              Collections
            </h2>
            <div
              className="grid gap-5"
              style={{ gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))" }}
            >
              {packs.map((pack) => {
                const saved = state.collections.some((collection) => collection.packId === pack.id);
                return (
                  <article key={pack.id} className="group/pack">
                    <div className="grid aspect-[16/10] grid-cols-3 grid-rows-2 gap-0.5 overflow-hidden rounded-panel bg-surface [&_img]:transition-[opacity,transform] [&_img]:duration-500 [&_img]:ease-quiet group-hover/pack:[&_img]:scale-[1.03]">
                      {pack.covers.slice(0, 3).map((cover, index) => (
                        <Picture
                          key={cover}
                          src={fileUrl(cover)}
                          alt=""
                          className={cn(
                            index === 0 &&
                              (pack.covers.length >= 3 ? "col-span-2 row-span-2" : "col-span-3 row-span-2"),
                          )}
                        />
                      ))}
                    </div>
                    <div className="mt-2.5 flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <h3 className="truncate font-medium">{pack.name}</h3>
                        <p className="line-clamp-2 text-sm text-fg-muted">{pack.description}</p>
                      </div>
                      <Button
                        size="sm"
                        disabled={saved || savingPack === pack.id}
                        onClick={() => savePack(pack)}
                        className="mt-0.5"
                      >
                        {saved ? (
                          <>
                            <Check />
                            Saved
                          </>
                        ) : savingPack === pack.id ? (
                          "Saving…"
                        ) : (
                          `Save all ${pack.count}`
                        )}
                      </Button>
                    </div>
                  </article>
                );
              })}
            </div>
          </section>
        )}

        {browsing && phase !== "error" && items.length > 0 && (
          <h2 className="mb-3 text-lg font-semibold">Wallpapers</h2>
        )}

        {phase === "loading" ? (
          <GridSkeleton count={12} />
        ) : phase === "error" ? (
          <EmptyState title="Discover is unavailable" body={`${error} Your library still works.`}>
            <Button variant="primary" onClick={() => setAttempt((count) => count + 1)}>
              Try again
            </Button>
          </EmptyState>
        ) : items.length === 0 ? (
          <EmptyState
            title={term ? `Nothing matches “${term}”` : "Nothing here yet"}
            body={term ? "Try a shorter or different word." : "New wallpapers are added regularly."}
          >
            {term && <Button onClick={() => setSearch("")}>Clear search</Button>}
          </EmptyState>
        ) : (
          <WallpaperGrid items={items} keyOf={(item) => item.id} aspectOf={itemAspect}>
            {(item, index) => {
              const saved = savedRemoteIds.has(item.id);
              const busy = busyRemoteIds.has(item.id);
              const saveButton = (
                <Button
                  variant="glass"
                  size="sm"
                  disabled={busy}
                  aria-pressed={saved}
                  onClick={() => toggleSave(item)}
                >
                  <SaveMark saved={saved} />
                  {busy ? "Saving…" : saved ? "Saved" : "Save"}
                </Button>
              );
              return (
                <WithContextMenu entries={itemMenu(item)}>
                  <div className="size-full">
                    <WallpaperTile
                      title={item.title}
                      caption={
                        // Said only when the picture really has the pixels to
                        // stretch across the displays connected right now.
                        item.kind === "image" && canSpan(item, displays)
                          ? `Spans ${plural(displays.length, "display")}`
                          : item.author
                      }
                      lowResolution={displays.length > 0 && !fitsAny(item, displays)}
                      thumb={fileUrl(item.files.thumb)}
                      color={item.color}
                      kind={item.kind}
                      liveSrc={item.kind === "image" ? undefined : fileUrl(item.files.original)}
                      onOpen={() => setViewing(index)}
                      // Saved stays visible so the state is clear without hovering.
                      {...(saved || busy ? { pinnedActions: saveButton } : { actions: saveButton })}
                    />
                  </div>
                </WithContextMenu>
              );
            }}
          </WallpaperGrid>
        )}

        <div ref={sentinel} className="h-px" />
        {phase === "more" && <p className="mt-6 text-center text-sm text-fg-muted">Loading more…</p>}
      </div>

      <Lightbox
        items={viewerItems}
        index={viewing}
        onIndexChange={setViewing}
        onClose={() => setViewing(null)}
        actions={(viewer) => {
          const item = items.find((candidate) => candidate.id === viewer.key);
          if (!item) return null;
          const saved = savedRemoteIds.has(item.id);
          const busy = busyRemoteIds.has(item.id);
          return (
            <>
              <Button size="lg" disabled={busy} aria-pressed={saved} onClick={() => toggleSave(item)}>
                <SaveMark saved={saved} />
                {busy ? "Saving…" : saved ? "Saved" : "Save"}
              </Button>
              <SetWallpaperButton
                size="lg"
                busy={setting}
                item={{ ...item, perDisplay: isPerDisplay(item) }}
                picture={{ src: fileUrl(item.files.thumb), color: item.color }}
                // Setting a wallpaper keeps it: it is saved to the library first.
                onSet={(target) => setWallpaper(() => library.saveItem(item), target)}
              />
            </>
          );
        }}
      />
    </div>
  );
}
