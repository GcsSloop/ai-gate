import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const isDesktopBuild = mode === "desktop" || process.env.DESKTOP_BUILD === "1";

  return {
    base: isDesktopBuild ? "./" : "/ai-router/webui/",
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
