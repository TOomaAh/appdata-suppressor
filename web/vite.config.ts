import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  resolve: {
    alias: { $lib: path.resolve("./src/lib") },
  },
  server: {
    // `npm run dev` proxies the API to a locally running Go server.
    proxy: { "/api": "http://localhost:8765" },
  },
});
