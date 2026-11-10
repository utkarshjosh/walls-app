import { forwardRef, type ButtonHTMLAttributes } from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-control font-medium transition-colors duration-100 disabled:pointer-events-none disabled:opacity-45 [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        // One filled action per view; everything else stays quiet.
        primary: "bg-tint text-tint-ink hover:bg-[color-mix(in_oklab,var(--tint)_88%,white)]",
        secondary:
          "bg-raised text-fg hover:bg-hover shadow-[inset_0_0_0_1px_var(--color-line)]",
        ghost: "text-fg-muted hover:bg-raised hover:text-fg",
        danger: "bg-danger/15 text-danger hover:bg-danger/25",
        // Sits on top of a picture.
        glass:
          "bg-black/45 text-white backdrop-blur-md hover:bg-black/65 shadow-[inset_0_0_0_1px_rgb(255_255_255/0.12)]",
      },
      size: {
        sm: "h-7 px-2.5 text-sm",
        md: "h-8 px-3 text-sm",
        lg: "h-10 px-4 text-base",
        icon: "size-8",
        "icon-sm": "size-7",
      },
    },
    defaultVariants: { variant: "secondary", size: "md" },
  },
);

export interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild, type, ...props }, ref) => {
    const Component = asChild ? Slot : "button";
    return (
      <Component
        ref={ref}
        type={asChild ? undefined : (type ?? "button")}
        className={cn(buttonVariants({ variant, size }), className)}
        {...props}
      />
    );
  },
);
Button.displayName = "Button";
