import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import {
  ContextMenu,
  ContextMenuCheckboxItem,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "./ui/menu";

// Actions are described once and rendered two ways: as a right-click menu and
// as a menu behind a visible button. Nothing is reachable by right-click only.

export type MenuEntry =
  | { type: "item"; label: string; icon?: LucideIcon; danger?: boolean; onSelect: () => void }
  | { type: "check"; label: string; checked: boolean; onChange: (checked: boolean) => void }
  | { type: "sub"; label: string; icon?: LucideIcon; entries: MenuEntry[] }
  | { type: "label"; label: string }
  | { type: "separator" };

function contextEntries(entries: MenuEntry[]): ReactNode {
  return entries.map((entry, index) => {
    switch (entry.type) {
      case "separator":
        return <ContextMenuSeparator key={index} />;
      case "label":
        return <ContextMenuLabel key={index}>{entry.label}</ContextMenuLabel>;
      case "check":
        return (
          <ContextMenuCheckboxItem
            key={index}
            checked={entry.checked}
            onCheckedChange={(value) => entry.onChange(value === true)}
          >
            {entry.label}
          </ContextMenuCheckboxItem>
        );
      case "sub":
        return (
          <ContextMenuSub key={index}>
            <ContextMenuSubTrigger>
              {entry.icon && <entry.icon />}
              {entry.label}
            </ContextMenuSubTrigger>
            <ContextMenuSubContent>{contextEntries(entry.entries)}</ContextMenuSubContent>
          </ContextMenuSub>
        );
      case "item":
        return (
          <ContextMenuItem key={index} danger={entry.danger} onSelect={entry.onSelect}>
            {entry.icon && <entry.icon />}
            {entry.label}
          </ContextMenuItem>
        );
    }
  });
}

function dropdownEntries(entries: MenuEntry[]): ReactNode {
  return entries.map((entry, index) => {
    switch (entry.type) {
      case "separator":
        return <DropdownMenuSeparator key={index} />;
      case "label":
        return <DropdownMenuLabel key={index}>{entry.label}</DropdownMenuLabel>;
      case "check":
        return (
          <DropdownMenuCheckboxItem
            key={index}
            checked={entry.checked}
            onCheckedChange={(value) => entry.onChange(value === true)}
          >
            {entry.label}
          </DropdownMenuCheckboxItem>
        );
      case "sub":
        return (
          <DropdownMenuSub key={index}>
            <DropdownMenuSubTrigger>
              {entry.icon && <entry.icon />}
              {entry.label}
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent>{dropdownEntries(entry.entries)}</DropdownMenuSubContent>
          </DropdownMenuSub>
        );
      case "item":
        return (
          <DropdownMenuItem key={index} danger={entry.danger} onSelect={entry.onSelect}>
            {entry.icon && <entry.icon />}
            {entry.label}
          </DropdownMenuItem>
        );
    }
  });
}

/** Wraps its child so right-clicking it opens the menu. */
export function WithContextMenu({ entries, children }: { entries: MenuEntry[]; children: ReactNode }) {
  if (entries.length === 0) return <>{children}</>;
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent>{contextEntries(entries)}</ContextMenuContent>
    </ContextMenu>
  );
}

/** A button (passed as `trigger`) that opens the same menu. */
export function ActionMenu({
  entries,
  trigger,
  align = "end",
  side,
}: {
  entries: MenuEntry[];
  trigger: ReactNode;
  align?: "start" | "center" | "end";
  side?: "top" | "bottom";
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      <DropdownMenuContent align={align} side={side}>
        {dropdownEntries(entries)}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
