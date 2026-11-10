import { forwardRef, type ComponentProps } from "react";
import * as SelectPrimitive from "@radix-ui/react-select";
import * as SwitchPrimitive from "@radix-ui/react-switch";
import { Check, ChevronsUpDown } from "lucide-react";
import { cn } from "@/lib/utils";

// Labelled controls, after Shadcn Space "Select 01" and "Switch 05" on Radix.

export function Label({ className, ...props }: ComponentProps<"label">) {
  return <label className={cn("text-sm text-fg-muted", className)} {...props} />;
}

const field =
  "h-8 rounded-control bg-raised px-2.5 text-sm text-fg shadow-[inset_0_0_0_1px_var(--color-line)] transition-shadow duration-100 placeholder:text-fg-faint hover:shadow-[inset_0_0_0_1px_var(--color-line-strong)] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_1.5px_var(--tint)] disabled:opacity-45";

export const Input = forwardRef<HTMLInputElement, ComponentProps<"input">>(
  ({ className, ...props }, ref) => (
    <input ref={ref} className={cn(field, "w-full", className)} {...props} />
  ),
);
Input.displayName = "Input";

export const Select = SelectPrimitive.Root;
export const SelectValue = SelectPrimitive.Value;

export function SelectTrigger({
  className,
  children,
  ...props
}: ComponentProps<typeof SelectPrimitive.Trigger>) {
  return (
    <SelectPrimitive.Trigger
      className={cn(field, "inline-flex items-center justify-between gap-2", className)}
      {...props}
    >
      {children}
      <SelectPrimitive.Icon>
        <ChevronsUpDown className="size-3.5 text-fg-faint" />
      </SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
  );
}

export function SelectContent({
  className,
  children,
  ...props
}: ComponentProps<typeof SelectPrimitive.Content>) {
  return (
    <SelectPrimitive.Portal>
      <SelectPrimitive.Content
        position="popper"
        sideOffset={6}
        className={cn(
          "z-50 max-h-[min(320px,var(--radix-select-content-available-height))] min-w-[var(--radix-select-trigger-width)] overflow-hidden rounded-[12px] bg-raised shadow-pop data-[state=open]:animate-pop-in",
          className,
        )}
        {...props}
      >
        <SelectPrimitive.Viewport className="p-1">{children}</SelectPrimitive.Viewport>
      </SelectPrimitive.Content>
    </SelectPrimitive.Portal>
  );
}

export function SelectItem({
  className,
  children,
  ...props
}: ComponentProps<typeof SelectPrimitive.Item>) {
  return (
    <SelectPrimitive.Item
      className={cn(
        "relative flex h-8 items-center rounded-[8px] pr-8 pl-2.5 text-sm outline-none select-none data-[highlighted]:bg-hover",
        className,
      )}
      {...props}
    >
      <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
      <SelectPrimitive.ItemIndicator className="absolute right-2.5">
        <Check className="size-4 text-tint" />
      </SelectPrimitive.ItemIndicator>
    </SelectPrimitive.Item>
  );
}

export function Switch({ className, ...props }: ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root
      className={cn(
        "relative inline-flex h-[22px] w-[38px] shrink-0 items-center rounded-full bg-line-strong transition-colors duration-150 data-[state=checked]:bg-tint disabled:opacity-45",
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb className="block size-[18px] translate-x-[2px] rounded-full bg-fg shadow-[0_1px_2px_rgb(0_0_0/0.4)] transition-transform duration-150 ease-quiet data-[state=checked]:translate-x-[18px] data-[state=checked]:bg-tint-ink" />
    </SwitchPrimitive.Root>
  );
}
