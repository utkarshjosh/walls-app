import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const root = path.dirname(fileURLToPath(import.meta.url));

/**
 * Development only: prints errors from the running page in this terminal.
 * The Mac app's window has no visible browser console, so without this a
 * crash inside it would go unseen. Nothing here exists in a production build.
 */
function reportPageErrors() {
  return {
    name: "still:report-page-errors",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use("/__page-report", (request, response) => {
        let body = "";
        request.on("data", (chunk) => (body += chunk));
        request.on("end", () => {
          server.config.logger.info(`[page] ${body.slice(0, 2000)}`, { timestamp: true });
          response.statusCode = 204;
          response.end();
        });
      });
    },
  };
}

export default defineConfig({
  // The desktop shell (apps/desktop) loads this dev server on a fixed port.
  clearScreen: false,
  envPrefix: ["VITE_", "TAURI_ENV_"],
  resolve: {
    alias: { "@": path.join(root, "src") },
  },
  build: {
    outDir: "dist/client",
    target: "es2022",
    // One bundle is fine: the Mac app loads it from disk, not over a network.
    chunkSizeWarningLimit: 900,
  },
  optimizeDeps: {
    include: ["react", "react-dom/client"],
  },
  server: {
    host: "0.0.0.0",
    port: 5173,
    strictPort: true,
    allowedHosts: ["terminal.local"],
    warmup: {
      clientFiles: ["./src/main.tsx"],
    },
  },
  plugins: [react(), tailwindcss(), reportPageErrors()],
});
