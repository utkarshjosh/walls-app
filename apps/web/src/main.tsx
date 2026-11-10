import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles.css";

if (import.meta.env.DEV) {
  // See reportPageErrors in vite.config.mjs: make page errors visible in the
  // dev server's terminal, which matters most inside the Mac app's window.
  const report = (kind: string, detail: unknown) =>
    navigator.sendBeacon(
      "/__page-report",
      `${kind}: ${detail instanceof Error ? (detail.stack ?? detail.message) : String(detail)}`,
    );
  window.addEventListener("error", (event) => report("error", event.error ?? event.message));
  window.addEventListener("unhandledrejection", (event) => report("unhandled rejection", event.reason));
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
