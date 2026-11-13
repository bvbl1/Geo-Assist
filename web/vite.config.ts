import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
export default defineConfig({
  plugins: [react()],
  server: {
    host: "localhost",
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": { target: "http://127.0.0.1:8080", changeOrigin: false },
      "/health": "http://127.0.0.1:8080",
    },
  },
  build: { chunkSizeWarningLimit: 650 },
});
