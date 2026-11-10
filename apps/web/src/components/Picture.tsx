import { useEffect, useRef, useState, type CSSProperties } from "react";
import { cn } from "@/lib/utils";
import { platform } from "@/platform";
import type { WallpaperKind } from "@/platform/types";

/**
 * A picture that fades in once decoded. Its box is sized by the parent and
 * tinted with the picture's own colour first, so the grid never jumps or flashes.
 */
export function Picture({
  src,
  alt,
  color,
  className,
  imageClassName,
  style,
  eager,
}: {
  src: string | undefined;
  alt: string;
  color?: string;
  className?: string;
  imageClassName?: string;
  style?: CSSProperties;
  eager?: boolean;
}) {
  const [ready, setReady] = useState(false);
  const image = useRef<HTMLImageElement>(null);

  useEffect(() => {
    // Cached pictures can finish before React attaches onLoad.
    setReady(Boolean(image.current?.complete && image.current.naturalWidth > 0));
  }, [src]);

  return (
    <div
      className={cn("overflow-hidden bg-raised", className)}
      style={{ backgroundColor: color, ...style }}
    >
      {src && (
        <img
          ref={image}
          src={src}
          alt={alt}
          loading={eager ? "eager" : "lazy"}
          decoding="async"
          draggable={false}
          data-ready={ready}
          onLoad={() => setReady(true)}
          className={cn("fade-media size-full object-cover", imageClassName)}
        />
      )}
    </div>
  );
}

/**
 * A playing live wallpaper: a looping video, or an animated scene in a
 * locked-down frame (scripts only — no network, no access to the app).
 */
export function LiveFrame({
  kind,
  src,
  className,
}: {
  kind: WallpaperKind;
  src: string;
  className?: string;
}) {
  if (kind === "video") {
    return (
      <video
        src={src}
        muted
        loop
        autoPlay
        playsInline
        disablePictureInPicture
        className={cn("pointer-events-none size-full object-cover", className)}
      />
    );
  }
  return (
    <iframe
      src={src}
      title=""
      aria-hidden
      tabIndex={-1}
      // The Mac app gives scenes the strictest frame: scripts only, no origin
      // of their own. The browser preview adds allow-same-origin because some
      // embedded browsers (including the preview pane used during development)
      // draw fully isolated frames as blank; there the scene still comes from
      // a different origin than the app, so it cannot reach the app's data.
      sandbox={platform.kind === "desktop" ? "allow-scripts" : "allow-scripts allow-same-origin"}
      className={cn("pointer-events-none size-full border-0", className)}
    />
  );
}
