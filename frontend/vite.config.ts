import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const isDesktopBuild = mode === "desktop" || process.env.DESKTOP_BUILD === "1";
  const isServerBuild = mode === "server" || process.env.AI_GATE_SERVER_BUILD === "1";
  const webuiBase = isDesktopBuild ? "./" : isServerBuild ? "/ai-gate/webui/" : "/ai-router/webui/";
  const apiPrefix = isServerBuild ? "/ai-gate/api" : "/ai-router/api";

  return {
    base: webuiBase,
    plugins: [
      react(),
      {
        name: "ai-router-root-redirect",
        configureServer(server) {
          server.middlewares.use((req, res, next) => {
            if (req.url === "/") {
              res.statusCode = 302;
              res.setHeader("Location", webuiBase);
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
        [apiPrefix]: "http://127.0.0.1:6789",
      },
    },
    build: {
      chunkSizeWarningLimit: 1200,
      rollupOptions: {
        output: {
          manualChunks: {
            react: ["react", "react-dom", "react-router-dom"],
            antd: ["antd", "@ant-design/icons"],
            tauri: ["@tauri-apps/api"],
          },
        },
      },
    },
    test: {
      environment: "jsdom",
      setupFiles: "./src/vitest.setup.ts",
      globals: true,
    },
  };
});
