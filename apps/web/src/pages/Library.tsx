import { useMemo, useState, type DragEvent, type ReactNode } from "react";
import {
  ChevronLeft,
  Download,
  Ellipsis,
  FolderMinus,
  FolderPlus,
  ImagePlus,
  Monitor,
  Pencil,
  Search,
  Trash2,
} from "lucide-react";
import { plural } from "@/lib/utils";
import { platform } from "@/platform";
import { useLibrary } from "@/state/store";
import { aspectOf, type Collection, type Wallpaper } from "@/state/model";
import { ActionMenu, WithContextMenu, type MenuEntry } from "@/components/ActionMenu";
import { ConfirmDialog, NameDialog } from "@/components/Dialogs";
import { orderedDisplays } from "@/components/DisplayPreview";
import { ImportDialog } from "@/components/ImportDialog";
import { Lightbox, type LightboxItem } from "@/components/Lightbox";
import { Picture } from "@/components/Picture";
import { SetWallpaperButton } from "@/components/SetWallpaperButton";
import { useToast } from "@/components/Toast";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/form";
import { useSetWallpaper } from "@/components/useSetWallpaper";
import { GridSkeleton, WallpaperGrid, WallpaperTile } from "@/components/WallpaperGrid";

const ALL = "all";

type Naming = { kind: "new" } | { kind: "rename"; collection: Collection };
type Confirming =
  | { kind: "collection"; collection: Collection }
  | { kind: "wallpaper"; wallpaper: Wallpaper };

export function LibraryPage({
  openId,
  onOpenIdChange,
  onOpenDiscover,
}: {
  /** null = the overview of collections, "all" = every wallpaper, otherwise a collection id. */
  openId: string | null;
  onOpenIdChange: (id: string | null) => void;
  onOpenDiscover: () => void;
}) {
  const library = useLibrary();
  const { state, loaded, urlOf, displays, status } = library;
  const notify = useToast();
  const { set: setWallpaper, busy: setting } = useSetWallpaper();

  const [query, setQuery] = useState("");
  const [importing, setImporting] = useState<{ files?: File[] } | null>(null);
  const [naming, setNaming] = useState<Naming | null>(null);
  const [confirming, setConfirming] = useState<Confirming | null>(null);
  const [viewing, setViewing] = useState<number | null>(null);
  const [dropping, setDropping] = useState(false);

  const openCollection =
    openId && openId !== ALL ? state.collections.find((item) => item.id === openId) : undefined;
  // The open collection was deleted elsewhere (e.g. from its own menu): fall back to the overview.
  const view: "overview" | "all" | "collection" =
    openId === ALL ? "all" : openCollection ? "collection" : "overview";

  const wallpapers = useMemo(() => {
    const list =
      view === "all"
        ? library.allWallpapers
        : openCollection
          ? library.wallpapersIn(openCollection.id)
          : [];
    const needle = query.trim().toLowerCase();
    return needle ? list.filter((item) => item.title.toLowerCase().includes(needle)) : list;
  }, [view, openCollection, library, query]);

  const showingIds = useMemo(() => new Set(status.current.map((entry) => entry.itemId)), [status]);

  const open = (id: string | null) => {
    onOpenIdChange(id);
    setQuery("");
    setViewing(null);
  };

  // ── Menus ───────────────────────────────────────────────────────────────
  const collectionMenu = (collection: Collection): MenuEntry[] => [
    {
      type: "item",
      label: "Add wallpapers…",
      icon: ImagePlus,
      onSelect: () => {
        open(collection.id);
        setImporting({});
      },
    },
    {
      type: "check",
      label: "Use in rotation",
      checked: collection.inRotation,
      onChange: (value) => library.setInRotation(collection.id, value),
    },
    { type: "separator" },
    {
      type: "item",
      label: "Rename…",
      icon: Pencil,
      onSelect: () => setNaming({ kind: "rename", collection }),
    },
    {
      type: "item",
      label: "Delete collection…",
      icon: Trash2,
      danger: true,
      onSelect: () => setConfirming({ kind: "collection", collection }),
    },
  ];

  const wallpaperMenu = (wallpaper: Wallpaper): MenuEntry[] => {
    const multi = displays.length > 1;
    const setEntries: MenuEntry[] = multi
      ? [
          {
            type: "sub",
            label: "Set as wallpaper",
            icon: Monitor,
            entries: [
              {
                type: "item",
                label: "On every display",
                onSelect: () => setWallpaper(wallpaper, { mode: "same" }),
              },
              {
                type: "item",
                label: "Across all displays",
                onSelect: () => setWallpaper(wallpaper, { mode: "span" }),
              },
              { type: "separator" },
              { type: "label", label: "Only on" },
              ...orderedDisplays(displays).map(
                (display): MenuEntry => ({
                  type: "item",
                  label: display.name,
                  onSelect: () => setWallpaper(wallpaper, { mode: "same", displayId: display.id }),
                }),
              ),
            ],
          },
        ]
      : [
          {
            type: "item",
            label: "Set as wallpaper",
            icon: Monitor,
            onSelect: () => setWallpaper(wallpaper),
          },
        ];
    const others = state.collections.filter((item) => item.id !== openCollection?.id);
    return [
      ...setEntries,
      ...(others.length
        ? [
            {
              type: "sub" as const,
              label: "Add to collection",
              icon: FolderPlus,
              entries: others.map(
                (collection): MenuEntry => ({
                  type: "check",
                  label: collection.name,
                  checked: collection.wallpaperIds.includes(wallpaper.id),
                  onChange: (value) =>
                    value
                      ? library.addToCollection(wallpaper.id, collection.id)
                      : library.removeFromCollection(wallpaper.id, collection.id),
                }),
              ),
            },
          ]
        : []),
      ...(wallpaper.kind === "image" && platform.kind === "web"
        ? [
            {
              type: "item" as const,
              label: "Download",
              icon: Download,
              onSelect: () => download(wallpaper, urlOf(wallpaper.full)),
            },
          ]
        : []),
      { type: "separator" },
      ...(openCollection
        ? [
            {
              type: "item" as const,
              label: `Remove from ${openCollection.name}`,
              icon: FolderMinus,
              onSelect: () => {
                library.removeFromCollection(wallpaper.id, openCollection.id);
                setViewing(null);
              },
            },
          ]
        : []),
      {
        type: "item",
        label: "Delete…",
        icon: Trash2,
        danger: true,
        onSelect: () => setConfirming({ kind: "wallpaper", wallpaper }),
      },
    ];
  };

  // ── Dropping files anywhere on the page ─────────────────────────────────
  const hasFiles = (event: DragEvent) => event.dataTransfer.types.includes("Files");
  const dropHandlers = {
    onDragOver: (event: DragEvent) => {
      if (!hasFiles(event) || importing) return;
      event.preventDefault();
      setDropping(true);
    },
    onDragLeave: (event: DragEvent) => {
      if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDropping(false);
    },
    onDrop: (event: DragEvent) => {
      if (!hasFiles(event) || importing) return;
      event.preventDefault();
      setDropping(false);
      const files = Array.from(event.dataTransfer.files);
      if (files.length) setImporting({ files });
    },
  };

  const viewerItems: LightboxItem[] = wallpapers.map((wallpaper) => ({
    key: wallpaper.id,
    title: wallpaper.title,
    kind: wallpaper.kind,
    image: urlOf(wallpaper.kind === "image" ? wallpaper.full : (wallpaper.poster ?? wallpaper.thumb)),
    live: wallpaper.kind === "image" ? undefined : urlOf(wallpaper.full),
    color: wallpaper.color,
    aspect: aspectOf(wallpaper),
    detail: describe(wallpaper),
  }));

  return (
    <div className="scroll-area relative h-full" {...dropHandlers}>
      <div className="mx-auto max-w-[1320px] px-8 pt-7 pb-16">
        {library.storageError && (
          <p className="mb-5 rounded-control bg-danger/10 px-3 py-2 text-sm text-danger" role="status">
            {library.storageError}
          </p>
        )}

        {view === "overview" ? (
          <>
            <PageHeader title="Library">
              <Button variant="ghost" onClick={() => setNaming({ kind: "new" })}>
                <FolderPlus />
                New collection
              </Button>
              <Button variant="primary" onClick={() => setImporting({})}>
                <ImagePlus />
                Add wallpapers
              </Button>
            </PageHeader>

            {!loaded ? (
              <GridSkeleton count={4} />
            ) : (
              <>
                {library.allWallpapers.length === 0 && (
                  <EmptyState
                    title="Nothing here yet"
                    body="Add pictures or videos from this Mac, or pick a few from Discover."
                  >
                    <Button variant="primary" onClick={() => setImporting({})}>
                      <ImagePlus />
                      Add wallpapers
                    </Button>
                    <Button onClick={onOpenDiscover}>Open Discover</Button>
                  </EmptyState>
                )}
                <div
                  className="grid gap-x-5 gap-y-7"
                  style={{ gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))" }}
                >
                  {library.allWallpapers.length > 0 && (
                    <CollectionCard
                      name="All wallpapers"
                      wallpapers={library.allWallpapers}
                      onOpen={() => open(ALL)}
                    />
                  )}
                  {state.collections.map((collection) => (
                    <WithContextMenu key={collection.id} entries={collectionMenu(collection)}>
                      <div>
                        <CollectionCard
                          name={collection.name}
                          wallpapers={library.wallpapersIn(collection.id)}
                          onOpen={() => open(collection.id)}
                          menu={
                            <ActionMenu
                              entries={collectionMenu(collection)}
                              trigger={
                                <Button
                                  variant="ghost"
                                  size="icon-sm"
                                  aria-label={`Actions for ${collection.name}`}
                                >
                                  <Ellipsis />
                                </Button>
                              }
                            />
                          }
                        />
                      </div>
                    </WithContextMenu>
                  ))}
                </div>
              </>
            )}
          </>
        ) : (
          <>
            <button
              type="button"
              onClick={() => open(null)}
              className="-ml-1.5 mb-2 inline-flex h-7 items-center gap-0.5 rounded-control pr-2 pl-1 text-sm text-fg-muted hover:bg-raised hover:text-fg"
            >
              <ChevronLeft className="size-4" />
              Library
            </button>
            <PageHeader
              title={openCollection?.name ?? "All wallpapers"}
              note={plural(
                (openCollection ? library.wallpapersIn(openCollection.id) : library.allWallpapers)
                  .length,
                "wallpaper",
              )}
            >
              <div className="relative">
                <Search className="pointer-events-none absolute top-2 left-2.5 size-4 text-fg-faint" />
                <Input
                  type="search"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Find by name"
                  aria-label="Find a wallpaper by name"
                  className="w-44 pl-8"
                />
              </div>
              <Button variant="primary" onClick={() => setImporting({})}>
                <ImagePlus />
                Add wallpapers
              </Button>
              {openCollection && (
                <ActionMenu
                  entries={collectionMenu(openCollection).slice(1)}
                  trigger={
                    <Button variant="ghost" size="icon" aria-label="Collection actions">
                      <Ellipsis />
                    </Button>
                  }
                />
              )}
            </PageHeader>

            {wallpapers.length === 0 ? (
              query ? (
                <EmptyState title={`Nothing named “${query.trim()}”`} body="Try a different name.">
                  <Button onClick={() => setQuery("")}>Clear search</Button>
                </EmptyState>
              ) : (
                <EmptyState
                  title="This collection is empty"
                  body="Add pictures or videos from this Mac, or save some from Discover."
                >
                  <Button variant="primary" onClick={() => setImporting({})}>
                    <ImagePlus />
                    Add wallpapers
                  </Button>
                  <Button onClick={onOpenDiscover}>Open Discover</Button>
                </EmptyState>
              )
            ) : (
              <WallpaperGrid items={wallpapers} keyOf={(item) => item.id} aspectOf={aspectOf}>
                {(wallpaper, index) => (
                  <WithContextMenu entries={wallpaperMenu(wallpaper)}>
                    <div className="size-full">
                      <WallpaperTile
                        title={wallpaper.title}
                        thumb={urlOf(wallpaper.thumb)}
                        color={wallpaper.color}
                        kind={wallpaper.kind}
                        liveSrc={wallpaper.kind === "image" ? undefined : urlOf(wallpaper.full)}
                        showing={showingIds.has(wallpaper.id)}
                        onOpen={() => setViewing(index)}
                        actions={
                          <>
                            {/* The most common action is one click away. */}
                            <Button
                              variant="glass"
                              size="sm"
                              disabled={setting}
                              onClick={() => setWallpaper(wallpaper)}
                            >
                              <Monitor />
                              Set
                              <span className="sr-only"> {wallpaper.title} as wallpaper</span>
                            </Button>
                            <ActionMenu
                              entries={wallpaperMenu(wallpaper)}
                              trigger={
                                <Button
                                  variant="glass"
                                  size="icon-sm"
                                  aria-label={`Actions for ${wallpaper.title}`}
                                >
                                  <Ellipsis />
                                </Button>
                              }
                            />
                          </>
                        }
                      />
                    </div>
                  </WithContextMenu>
                )}
              </WallpaperGrid>
            )}
            {wallpapers.some((item) => item.origin === "sample") && (
              <p className="mt-8 text-xs text-fg-faint">
                Sample images are included to get you started. Delete them whenever you like.
              </p>
            )}
          </>
        )}
      </div>

      {dropping && (
        <div className="pointer-events-none absolute inset-3 z-30 flex items-center justify-center rounded-dialog border-2 border-dashed border-tint bg-bg/80 backdrop-blur-sm">
          <p className="text-lg font-medium">
            Drop to add to {openCollection ? openCollection.name : "your library"}
          </p>
        </div>
      )}

      <Lightbox
        items={viewerItems}
        index={viewing}
        onIndexChange={setViewing}
        onClose={() => setViewing(null)}
        actions={(item) => {
          const wallpaper = state.wallpapers[item.key];
          if (!wallpaper) return null;
          return (
            <>
              <ActionMenu
                side="top"
                entries={wallpaperMenu(wallpaper).filter(
                  (entry) => !("label" in entry && entry.label === "Set as wallpaper"),
                )}
                trigger={
                  <Button variant="ghost" size="icon" aria-label="More actions">
                    <Ellipsis />
                  </Button>
                }
              />
              <SetWallpaperButton
                size="lg"
                busy={setting}
                picture={{ src: item.image, color: item.color }}
                onSet={(target) => setWallpaper(wallpaper, target)}
              />
            </>
          );
        }}
      />

      <ImportDialog
        open={Boolean(importing)}
        onClose={() => setImporting(null)}
        collectionId={openCollection?.id ?? null}
        initialFiles={importing?.files}
      />

      <NameDialog
        open={Boolean(naming)}
        title={naming?.kind === "rename" ? "Rename collection" : "New collection"}
        initial={naming?.kind === "rename" ? naming.collection.name : ""}
        actionLabel={naming?.kind === "rename" ? "Rename" : "Create"}
        onClose={() => setNaming(null)}
        onSubmit={(name) => {
          if (naming?.kind === "rename") {
            const result = library.renameCollection(naming.collection.id, name);
            return "error" in result ? result.error : null;
          }
          const result = library.createCollection(name);
          if ("error" in result) return result.error;
          open(result.id);
          return null;
        }}
      />

      <ConfirmDialog
        open={Boolean(confirming)}
        onClose={() => setConfirming(null)}
        title={
          confirming?.kind === "collection"
            ? `Delete “${confirming.collection.name}”?`
            : `Delete “${confirming?.wallpaper.title ?? ""}”?`
        }
        description={
          confirming?.kind === "collection"
            ? "The collection goes away. Its wallpapers stay in your library."
            : "It is removed from your library and every collection. This can't be undone."
        }
        actionLabel={confirming?.kind === "collection" ? "Delete collection" : "Delete wallpaper"}
        onConfirm={() => {
          if (!confirming) return;
          if (confirming.kind === "collection") {
            library.deleteCollection(confirming.collection.id);
          } else {
            setViewing(null);
            void library.deleteWallpapers([confirming.wallpaper.id]);
            notify("Wallpaper deleted");
          }
        }}
      />
    </div>
  );
}

function describe(wallpaper: Wallpaper): string {
  const size = wallpaper.width && wallpaper.height ? `${wallpaper.width} × ${wallpaper.height}` : "";
  const source =
    wallpaper.origin === "sample"
      ? "sample image"
      : wallpaper.origin === "discover" && wallpaper.author
        ? `by ${wallpaper.author}`
        : wallpaper.kind === "video"
          ? "your video"
          : wallpaper.origin === "import"
            ? "added by you"
            : "";
  return [size, source].filter(Boolean).join(", ");
}

function download(wallpaper: Wallpaper, url: string | undefined) {
  if (!url) return;
  const link = document.createElement("a");
  link.href = url;
  link.download = `${wallpaper.title}.jpg`;
  link.click();
}

export function PageHeader({
  title,
  note,
  children,
}: {
  title: string;
  note?: string;
  children?: ReactNode;
}) {
  return (
    <header className="mb-6 flex min-h-9 flex-wrap items-center justify-between gap-x-6 gap-y-3">
      <div className="flex min-w-0 items-baseline gap-3">
        <h1 className="truncate text-2xl font-semibold tracking-[-0.02em]">{title}</h1>
        {note && <span className="shrink-0 text-sm text-fg-muted">{note}</span>}
      </div>
      {children && <div className="flex items-center gap-2">{children}</div>}
    </header>
  );
}

export function EmptyState({
  title,
  body,
  children,
}: {
  title: string;
  body: string;
  children?: ReactNode;
}) {
  return (
    <div className="mb-8 flex flex-col items-center rounded-panel border border-dashed border-line px-6 py-14 text-center">
      <h2 className="text-lg font-semibold">{title}</h2>
      <p className="mt-1 max-w-sm text-sm text-fg-muted">{body}</p>
      {children && <div className="mt-5 flex gap-2">{children}</div>}
    </div>
  );
}

function CollectionCard({
  name,
  wallpapers,
  onOpen,
  menu,
}: {
  name: string;
  wallpapers: Wallpaper[];
  onOpen: () => void;
  menu?: ReactNode;
}) {
  const { urlOf } = useLibrary();
  // Spread the three cover pictures across the set so the cover hints at its range.
  const cover =
    wallpapers.length <= 3
      ? wallpapers
      : [0, 1, 2].map((slot) => wallpapers[Math.floor((slot * wallpapers.length) / 3)]!);
  return (
    <div className="group/card">
      <button
        type="button"
        onClick={onOpen}
        aria-label={`Open ${name}`}
        className="block aspect-[16/10] w-full overflow-hidden rounded-panel bg-surface"
      >
        {cover.length === 0 ? (
          <span className="flex size-full items-center justify-center border border-dashed border-line text-sm text-fg-faint">
            Empty
          </span>
        ) : cover.length < 3 ? (
          <Picture
            src={urlOf(cover[0]!.thumb)}
            alt=""
            color={cover[0]!.color}
            className="size-full"
          />
        ) : (
          // One large picture and two small: reads as "a set" at a glance.
          <span className="grid size-full grid-cols-3 grid-rows-2 gap-0.5">
            <Picture
              src={urlOf(cover[0]!.thumb)}
              alt=""
              color={cover[0]!.color}
              className="col-span-2 row-span-2"
            />
            <Picture src={urlOf(cover[1]!.thumb)} alt="" color={cover[1]!.color} />
            <Picture src={urlOf(cover[2]!.thumb)} alt="" color={cover[2]!.color} />
          </span>
        )}
      </button>
      <div className="mt-2.5 flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate font-medium">{name}</p>
          <p className="text-sm text-fg-muted">{plural(wallpapers.length, "wallpaper")}</p>
        </div>
        {menu && (
          <span className="opacity-0 transition-opacity duration-150 group-focus-within/card:opacity-100 group-hover/card:opacity-100 has-[[data-state=open]]:opacity-100">
            {menu}
          </span>
        )}
      </div>
    </div>
  );
}
