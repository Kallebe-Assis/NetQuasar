import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  /** Evita duas cópias do Leaflet (quebrava `icon.createIcon` com react-leaflet + chunks). */
  resolve: {
    dedupe: ["leaflet", "react-leaflet"],
  },
  optimizeDeps: {
    /** Não empacotar react-leaflet à parte com outra cópia do Leaflet (dev). */
    include: ["leaflet"],
    exclude: ["react-leaflet"],
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
    },
  },
});
