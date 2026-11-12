import { defineConfig } from "vite";

// The landing page is plain HTML, CSS and TypeScript. three.js is loaded as its
// own chunk after first paint, so the text is readable before the 3D arrives.
const toService = { "/v1": "http://localhost:8787", "/admin": "http://localhost:8787" };

export default defineConfig({
  // The hero compiles scene shaders straight from the catalog's source folder.
  // On the server the page and the catalog service share one address. Here
  // they do not, so requests the page makes to the service (the list of
  // builds, the downloads themselves, the visit count) are passed on to a
  // service running on this machine.
  server: { port: 5180, strictPort: true, fs: { allow: [".", "../api/seed/scene-sources"] }, proxy: toService },
  preview: { port: 5180, strictPort: true, proxy: toService },
  // Two pages: the landing page, and the plain page that says what is recorded.
  build: { target: "es2022", chunkSizeWarningLimit: 900, rollupOptions: { input: ["index.html", "privacy.html"] } },
});
