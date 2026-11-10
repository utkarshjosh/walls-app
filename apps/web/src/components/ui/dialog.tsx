import type { ComponentProps, ReactNode } from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "./button";

// Short forms and confirmations. Radix supplies focus trapping, Escape, and
// returning focus to whatever opened the dialog.

export const Dialog = DialogPrimitive.Root;
export const DialogClose = DialogPrimitive.Close;

export function DialogContent({
  className,
  children,
  title,
  description,
  ...props
}: Omit<ComponentProps<typeof DialogPrimitive.Content>, "title"> & {
  title: ReactNode;
  description?: ReactNode;
}) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="fixed inset-0 z-40 bg-scrim data-[state=closed]:animate-fade-out data-[state=open]:animate-fade-in" />
      <DialogPrimitive.Content
        className={cn(
          "fixed top-1/2 left-1/2 z-50 flex max-h-[calc(100vh-48px)] w-[min(440px,calc(100vw-32px))] -translate-x-1/2 -translate-y-1/2 flex-col rounded-dialog bg-surface p-5 shadow-pop",
          "data-[state=closed]:animate-fade-out data-[state=open]:animate-fade-in",
          className,
        )}
        {...(description ? {} : { "aria-describedby": undefined })}
        {...props}
      >
        <div className="mb-4 flex items-start justify-between gap-4">
          <div className="min-w-0">
            <DialogPrimitive.Title className="text-lg font-semibold tracking-[-0.01em]">
              {title}
            </DialogPrimitive.Title>
            {description && (
              <DialogPrimitive.Description className="mt-1 text-sm text-fg-muted">
                {description}
              </DialogPrimitive.Description>
            )}
          </div>
          <DialogPrimitive.Close asChild>
            <Button variant="ghost" size="icon-sm" aria-label="Close" className="-mt-1 -mr-1.5">
              <X />
            </Button>
          </DialogPrimitive.Close>
        </div>
        {children}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
}

export function DialogActions({ className, ...props }: ComponentProps<"div">) {
  return <div className={cn("mt-5 flex justify-end gap-2", className)} {...props} />;
}
