import { useState } from "react";
import { ChevronDown, Monitor } from "lucide-react";
import { useLibrary, type WallpaperTarget } from "@/state/store";
import { Button } from "./ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./ui/menu";
import { DisplayPreview, ModeGlyph, orderedDisplays, type PreviewPicture } from "./DisplayPreview";

/**
 * "Set as wallpaper". With one display it is a single button. A second display
 * unlocks the arrow beside it: everywhere, spanned across, or one display only —
 * with a live miniature of the arrangement showing the result.
 */
export function SetWallpaperButton({
  onSet,
  picture,
  busy,
  size = "md",
}: {
  onSet: (target?: WallpaperTarget) => void;
  /** Shown on the miniature displays inside the menu. */
  picture: PreviewPicture;
  busy?: boolean;
  size?: "md" | "lg";
}) {
  const { displays } = useLibrary();
  const [hover, setHover] = useState<{ mode: "same" | "span"; displayId?: string }>({
    mode: "same",
  });
  const multi = displays.length > 1;

  const main = (
    <Button
      variant="primary"
      size={size}
      disabled={busy}
      onClick={() => onSet(multi ? { mode: "same" } : undefined)}
      className={multi ? "rounded-r-none" : undefined}
    >
      {busy ? "Setting…" : "Set as wallpaper"}
    </Button>
  );
  if (!multi) return main;

  return (
    <div className="flex">
      {main}
      <DropdownMenu onOpenChange={() => setHover({ mode: "same" })}>
        <DropdownMenuTrigger asChild>
          <Button
            variant="primary"
            size={size}
            disabled={busy}
            aria-label="Choose displays"
            className="rounded-l-none border-l border-tint-ink/25 px-1.5"
          >
            <ChevronDown />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" side="top" className="w-[264px]">
          <div className="px-3 pt-3 pb-2">
            <DisplayPreview
              displays={displays}
              mode={hover.mode}
              pictures={[picture]}
              highlightId={hover.displayId ?? null}
            />
          </div>
          <DropdownMenuItem
            onSelect={() => onSet({ mode: "same" })}
            onFocus={() => setHover({ mode: "same" })}
            onPointerMove={() => setHover({ mode: "same" })}
          >
            <ModeGlyph mode="same" className="text-fg-muted" />
            On every display
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() => onSet({ mode: "span" })}
            onFocus={() => setHover({ mode: "span" })}
            onPointerMove={() => setHover({ mode: "span" })}
          >
            <ModeGlyph mode="span" className="text-fg-muted" />
            Across all displays
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuLabel>Only on</DropdownMenuLabel>
          {orderedDisplays(displays).map((display) => (
            <DropdownMenuItem
              key={display.id}
              onSelect={() => onSet({ mode: "same", displayId: display.id })}
              onFocus={() => setHover({ mode: "same", displayId: display.id })}
              onPointerMove={() => setHover({ mode: "same", displayId: display.id })}
            >
              <Monitor />
              {display.name}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
