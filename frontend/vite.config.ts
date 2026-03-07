import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/accounts": "http://127.0.0.1:8080",
      "/policy": "http://127.0.0.1:8080",
      "/monitoring": "http://127.0.0.1:8080",
      "/conversations": "http://127.0.0.1:8080",
      "/v1": "http://127.0.0.1:8080",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/vitest.setup.ts",
    globals: true,
  },
});
