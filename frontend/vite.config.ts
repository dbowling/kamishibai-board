/// <reference types="vitest" />
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// The dev server proxies the API to the Go backend so the browser sees a single
// origin, which keeps cookies, realtime subscriptions and relative URLs working
// exactly as they do in production.
const backendAddress = process.env.KAMISHIBAI_HTTP_ADDR ?? '127.0.0.1:8090';
const backendURL = `http://${backendAddress}`;

export default defineConfig({
  plugins: [react()],

  build: {
    // The backend serves this directory as static files, so a production build
    // drops straight into place and there is nothing extra to deploy.
    outDir: '../backend/pb_public',
    emptyOutDir: true,
    sourcemap: true,
  },

  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: backendURL, changeOrigin: true, ws: true },
      // PocketBase's own dashboard, handy during development.
      '/_': { target: backendURL, changeOrigin: true },
    },
  },

  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/vitest.setup.ts'],
    css: false,
    restoreMocks: true,
  },
});
