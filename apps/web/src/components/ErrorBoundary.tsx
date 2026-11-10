import { Component, type ReactNode } from "react";

/**
 * If a screen fails while drawing, say so and offer a way back, rather than
 * leaving an empty window. The library on disk is not affected by a reload.
 */
export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <div
        data-tauri-drag-region
        className="flex h-full flex-col items-center justify-center gap-1 px-6 text-center"
      >
        <h1 className="text-lg font-semibold">Something went wrong</h1>
        <p className="max-w-sm text-sm text-fg-muted">
          Still hit a problem drawing this screen. Your wallpapers are safe.
        </p>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="mt-4 h-8 rounded-control bg-tint px-3 text-sm font-medium text-tint-ink"
        >
          Reload
        </button>
      </div>
    );
  }
}
