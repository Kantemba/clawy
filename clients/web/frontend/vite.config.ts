import path from "path"

import tailwindcss from "@tailwindcss/vite"
import { tanstackRouter } from "@tanstack/router-plugin/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, type ProxyOptions } from "vite"

const BACKEND_ORIGIN = "http://localhost:18800"

/**
 * Proxy an API path to the launcher backend.
 *
 * The launcher rejects requests whose `Origin` does not match its own host
 * (cross-site guard on `/api/auth/setup`). Through this dev proxy the browser
 * still sends the Vite origin (`http://localhost:5173`), so rewrite it to the
 * backend origin before forwarding.
 */
const backendApiProxy = (target: string): ProxyOptions => ({
  target,
  changeOrigin: true,
  configure: (proxy) => {
    const useBackendOrigin = (proxyReq: {
      setHeader: (name: string, value: string) => void
    }) => {
      proxyReq.setHeader("Origin", BACKEND_ORIGIN)
    }
    proxy.on("proxyReq", useBackendOrigin)
    proxy.on("proxyReqWs", useBackendOrigin)
  },
})

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    tanstackRouter({
      target: "react",
      autoCodeSplitting: true,
    }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    chunkSizeWarningLimit: 2048,
  },
  server: {
    proxy: {
      "/api": backendApiProxy(BACKEND_ORIGIN),
      "/pico/media": backendApiProxy(BACKEND_ORIGIN),
      "/pico/ws": {
        target: "ws://localhost:18800",
        ws: true,
      },
    },
  },
})
