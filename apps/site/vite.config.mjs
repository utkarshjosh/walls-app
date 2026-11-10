import { defineConfig } from "vite";

// The landing page is plain HTML, CSS and TypeScript. three.js is loaded as its
// own chunk after first paint, so the text is readable before the 3D arrives.
export default defineConfig({
  // The hero compiles scene shaders straight from the catalog's source folder.
  server: { port: 5180, strictPort: true, fs: { allow: [".", "../api/seed/scene-sources"] } },
  preview: { port: 5180, strictPort: true },
  build: { target: "es2022", chunkSizeWarningLimit: 900 },
});
