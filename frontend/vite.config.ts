import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "/ai-router/webui/",
  plugins: [
    react(),
    {
      name: "ai-router-root-redirect",
      configureServer(server) {
        server.middlewares.use((req, res, next) => {
          if (req.url === "/") {
            res.statusCode = 302;
            res.setHeader("Location", "/ai-router/webui/");
            res.end();
            return;
          }
          next();
        });
      },
    },
  ],
  server: {
    proxy: {
      "/ai-router/api": "http://127.0.0.1:6789",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/vitest.setup.ts",
    globals: true,
  },
});
