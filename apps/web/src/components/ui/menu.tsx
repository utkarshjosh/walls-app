import type { ComponentProps } from "react";
import * as ContextPrimitive from "@radix-ui/react-context-menu";
import * as DropdownPrimitive from "@radix-ui/react-dropdown-menu";
import { Check, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

// One menu look for right-click menus and button menus, so every action is
// reachable both ways (pointer and keyboard) and reads the same.
// Visual treatment adapted from Shadcn Space "Context Menu 01" on Radix menus.

const surface =
  "z-50 min-w-[190px] overflow-hidden rounded-[12px] bg-raised p-1 shadow-pop data-[state=open]:animate-pop-in";
const item =
  "relative flex h-8 items-center gap-2 rounded-[8px] px-2.5 text-sm outline-none select-none data-[disabled]:opacity-40 data-[highlighted]:bg-hover [&_svg]:size-4 [&_svg]:shrink-0 [&_svg]:text-fg-muted";
const dangerItem = "text-danger [&_svg]:text-danger data-[highlighted]:bg-danger/15";
const separator = "mx-1.5 my-1 h-px bg-line";
const label = "px-2.5 pt-1.5 pb-1 text-xs text-fg-faint";

// ── Right-click ────────────────────────────────────────────────────────────
export const ContextMenu = ContextPrimitive.Root;
export const ContextMenuTrigger = ContextPrimitive.Trigger;
export const ContextMenuSub = ContextPrimitive.Sub;

export function ContextMenuContent({
  className,
  ...props
}: ComponentProps<typeof ContextPrimitive.Content>) {
  return (
    <ContextPrimitive.Portal>
      <ContextPrimitive.Content className={cn(surface, className)} {...props} />
    </ContextPrimitive.Portal>
  );
}

export function ContextMenuItem({
  className,
  danger,
  ...props
}: ComponentProps<typeof ContextPrimitive.Item> & { danger?: boolean }) {
  return <ContextPrimitive.Item className={cn(item, danger && dangerItem, className)} {...props} />;
}

export function ContextMenuCheckboxItem({
  className,
  children,
  ...props
}: ComponentProps<typeof ContextPrimitive.CheckboxItem>) {
  return (
    <ContextPrimitive.CheckboxItem className={cn(item, "pr-8", className)} {...props}>
      {children}
      <ContextPrimitive.ItemIndicator className="absolute right-2.5">
        <Check />
      </ContextPrimitive.ItemIndicator>
    </ContextPrimitive.CheckboxItem>
  );
}

export function ContextMenuSubTrigger({
  className,
  children,
  ...props
}: ComponentProps<typeof ContextPrimitive.SubTrigger>) {
  return (
    <ContextPrimitive.SubTrigger
      className={cn(item, "data-[state=open]:bg-hover", className)}
      {...props}
    >
      {children}
      <ChevronRight className="ml-auto" />
    </ContextPrimitive.SubTrigger>
  );
}

export function ContextMenuSubContent({
  className,
  ...props
}: ComponentProps<typeof ContextPrimitive.SubContent>) {
  return (
    <ContextPrimitive.Portal>
      <ContextPrimitive.SubContent className={cn(surface, className)} sideOffset={6} {...props} />
    </ContextPrimitive.Portal>
  );
}

export const ContextMenuSeparator = (props: ComponentProps<typeof ContextPrimitive.Separator>) => (
  <ContextPrimitive.Separator className={separator} {...props} />
);
export const ContextMenuLabel = (props: ComponentProps<typeof ContextPrimitive.Label>) => (
  <ContextPrimitive.Label className={label} {...props} />
);

// ── Button menus ───────────────────────────────────────────────────────────
export const DropdownMenu = DropdownPrimitive.Root;
export const DropdownMenuTrigger = DropdownPrimitive.Trigger;
export const DropdownMenuSub = DropdownPrimitive.Sub;

export function DropdownMenuContent({
  className,
  sideOffset = 6,
  ...props
}: ComponentProps<typeof DropdownPrimitive.Content>) {
  return (
    <DropdownPrimitive.Portal>
      <DropdownPrimitive.Content
        sideOffset={sideOffset}
        className={cn(surface, className)}
        {...props}
      />
    </DropdownPrimitive.Portal>
  );
}

export function DropdownMenuItem({
  className,
  danger,
  ...props
}: ComponentProps<typeof DropdownPrimitive.Item> & { danger?: boolean }) {
  return <DropdownPrimitive.Item className={cn(item, danger && dangerItem, className)} {...props} />;
}

export function DropdownMenuCheckboxItem({
  className,
  children,
  ...props
}: ComponentProps<typeof DropdownPrimitive.CheckboxItem>) {
  return (
    <DropdownPrimitive.CheckboxItem className={cn(item, "pr-8", className)} {...props}>
      {children}
      <DropdownPrimitive.ItemIndicator className="absolute right-2.5">
        <Check />
      </DropdownPrimitive.ItemIndicator>
    </DropdownPrimitive.CheckboxItem>
  );
}

export function DropdownMenuSubTrigger({
  className,
  children,
  ...props
}: ComponentProps<typeof DropdownPrimitive.SubTrigger>) {
  return (
    <DropdownPrimitive.SubTrigger
      className={cn(item, "data-[state=open]:bg-hover", className)}
      {...props}
    >
      {children}
      <ChevronRight className="ml-auto" />
    </DropdownPrimitive.SubTrigger>
  );
}

export function DropdownMenuSubContent({
  className,
  ...props
}: ComponentProps<typeof DropdownPrimitive.SubContent>) {
  return (
    <DropdownPrimitive.Portal>
      <DropdownPrimitive.SubContent className={cn(surface, className)} sideOffset={6} {...props} />
    </DropdownPrimitive.Portal>
  );
}

export const DropdownMenuSeparator = (
  props: ComponentProps<typeof DropdownPrimitive.Separator>,
) => <DropdownPrimitive.Separator className={separator} {...props} />;
export const DropdownMenuLabel = (props: ComponentProps<typeof DropdownPrimitive.Label>) => (
  <DropdownPrimitive.Label className={label} {...props} />
);
