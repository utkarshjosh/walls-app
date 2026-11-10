import { useEffect, useState } from "react";
import * as Tabs from "@radix-ui/react-tabs";
import { MotionConfig, motion } from "motion/react";
import { Ellipsis, FlaskConical } from "lucide-react";
import { cn } from "@/lib/utils";
import { isMac, isWebPlatform, platform } from "@/platform";
import { LibraryProvider, useLibrary } from "@/state/store";
import { ToastProvider, useToast } from "@/components/Toast";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/menu";
import { DiscoverPage } from "@/pages/Discover";
import { LibraryPage } from "@/pages/Library";
import { RotationPage } from "@/pages/Rotation";

type Destination = "library" | "discover" | "rotation";

const DESTINATIONS: { id: Destination; label: string }[] = [
  { id: "library", label: "Library" },
  { id: "discover", label: "Discover" },
  { id: "rotation", label: "Rotation" },
];

const ACCENTS = [
  { id: "champagne", label: "Champagne" },
  { id: "sage", label: "Sage" },
  { id: "clay", label: "Clay" },
] as const;
const ACCENT_KEY = "still.preview.accent";

export function App() {
  return (
    <MotionConfig reducedMotion="user" transition={{ duration: 0.18, ease: [0.2, 0, 0, 1] }}>
      <ToastProvider>
        <LibraryProvider>
          <Shell />
        </LibraryProvider>
      </ToastProvider>
    </MotionConfig>
  );
}

function Shell() {
  const library = useLibrary();
  const notify = useToast();
  const [destination, setDestination] = useState<Destination>("library");
  // A screen is built the first time it is visited and then kept, so scroll
  // position, search text and loaded pages survive switching away and back.
  const [visited, setVisited] = useState<ReadonlySet<Destination>>(new Set(["library"]));
  const [libraryOpenId, setLibraryOpenId] = useState<string | null>(null);

  const go = (next: Destination) => {
    setDestination(next);
    setVisited((current) => (current.has(next) ? current : new Set(current).add(next)));
  };

  // ⌘1 / ⌘2 / ⌘3, as in any Mac app with a few top-level sections.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (!(event.metaKey || event.ctrlKey) || event.shiftKey || event.altKey) return;
      const target = DESTINATIONS[Number(event.key) - 1];
      if (target) {
        event.preventDefault();
        go(target.id);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // The moment a second display appears, say once what it unlocked.
  const { loaded, displays, state, markHintSeen } = library;
  useEffect(() => {
    if (!loaded || displays.length < 2 || state.hints.multiDisplay) return;
    markHintSeen("multiDisplay");
    notify(`${displays.length} displays found. One wallpaper can now stretch across them.`, {
      action: { label: "Show me", run: () => go("rotation") },
    });
  }, [loaded, displays.length, state.hints.multiDisplay, markHintSeen, notify]);

  const desktopMac = platform.kind === "desktop" && isMac;

  return (
    <Tabs.Root
      value={destination}
      onValueChange={(value) => go(value as Destination)}
      className="flex h-full flex-col"
    >
      <header
        data-tauri-drag-region
        className={cn(
          "flex h-[52px] shrink-0 items-stretch border-b border-line pr-3",
          // Leave room for the window's close/minimise/zoom buttons.
          desktopMac ? "pl-[92px]" : "pl-6",
        )}
      >
        {!desktopMac && (
          <span className="mr-7 flex items-center text-base font-semibold tracking-[-0.01em]">
            Still
          </span>
        )}
        {/* Underline navigation after Shadcn Space "Tabs 05": all three labels
            stay visible; only the underline moves. */}
        <Tabs.List aria-label="Sections" className="flex items-stretch gap-1">
          {DESTINATIONS.map((item) => {
            const active = item.id === destination;
            return (
              <Tabs.Trigger
                key={item.id}
                value={item.id}
                className={cn(
                  "relative px-3 text-base transition-colors duration-100 focus-visible:outline-offset-[-4px]",
                  active ? "font-medium text-fg" : "text-fg-muted hover:text-fg",
                )}
              >
                {item.label}
                {active && (
                  <motion.span
                    layoutId="section-underline"
                    className="absolute inset-x-3 -bottom-px h-0.5 rounded-full bg-tint"
                  />
                )}
              </Tabs.Trigger>
            );
          })}
        </Tabs.List>

        <div className="ml-auto flex items-center gap-1">
          {isWebPlatform(platform) && <PreviewTools />}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" aria-label="More">
                <Ellipsis />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={library.exportBackup}>Export library backup</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>

      <main className="min-h-0 flex-1">
        {DESTINATIONS.map((item) => (
          <Tabs.Content
            key={item.id}
            value={item.id}
            forceMount
            className="h-full focus-visible:outline-none data-[state=active]:animate-fade-in data-[state=inactive]:hidden"
          >
            {visited.has(item.id) &&
              (item.id === "library" ? (
                <LibraryPage
                  openId={libraryOpenId}
                  onOpenIdChange={setLibraryOpenId}
                  onOpenDiscover={() => go("discover")}
                />
              ) : item.id === "discover" ? (
                <DiscoverPage
                  onOpenCollection={(id) => {
                    setLibraryOpenId(id);
                    go("library");
                  }}
                />
              ) : (
                <RotationPage
                  onOpenLibrary={() => {
                    setLibraryOpenId(null);
                    go("library");
                  }}
                />
              ))}
          </Tabs.Content>
        ))}
      </main>
    </Tabs.Root>
  );
}

/**
 * Browser preview only — never part of the Mac app. Lets you try what the app
 * does when more displays are connected, and compare the three accent
 * proposals, without the hardware or a code change.
 */
function PreviewTools() {
  const web = isWebPlatform(platform) ? platform : null;
  const [count, setCount] = useState(() => web?.simulatedDisplayCount() ?? 1);
  const [accent, setAccent] = useState<string>(() => {
    try {
      return localStorage.getItem(ACCENT_KEY) ?? "champagne";
    } catch {
      return "champagne";
    }
  });

  useEffect(() => {
    const root = document.documentElement;
    if (accent === "champagne") delete root.dataset.accent;
    else root.dataset.accent = accent;
    try {
      localStorage.setItem(ACCENT_KEY, accent);
    } catch {
      /* preview-only preference */
    }
  }, [accent]);

  if (!web) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="text-fg-faint">
          <FlaskConical />
          Preview tools
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel>Pretend displays connected</DropdownMenuLabel>
        {[1, 2, 3].map((value) => (
          <DropdownMenuCheckboxItem
            key={value}
            checked={count === value}
            onCheckedChange={() => {
              web.setSimulatedDisplayCount(value);
              setCount(value);
            }}
          >
            {value === 1 ? "One display" : value === 2 ? "Two displays" : "Three displays"}
          </DropdownMenuCheckboxItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuLabel>Accent colour (not yet chosen)</DropdownMenuLabel>
        {ACCENTS.map((option) => (
          <DropdownMenuCheckboxItem
            key={option.id}
            checked={accent === option.id}
            onCheckedChange={() => setAccent(option.id)}
          >
            {option.label}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
