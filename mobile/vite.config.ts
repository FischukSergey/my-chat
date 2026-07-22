import { defineConfig } from "vite";

export default defineConfig({
  root: "src",
  envDir: "..",   // .env файлы лежат в mobile/, а не в mobile/src/
  build: {
    outDir: "../dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api/v1/auth": {
        target: "http://localhost:33081",
        changeOrigin: true,
      },
      "/api/v1": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
