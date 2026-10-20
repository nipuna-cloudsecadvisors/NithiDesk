import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { VitePWA } from 'vite-plugin-pwa';

// The Go API serves dist/ with a strict CSP (default-src 'self', no inline
// script or style), so nothing here may rely on inline code.
export default defineConfig({
  plugins: [
    react(),
    VitePWA({
      strategies: 'generateSW',
      registerType: 'prompt',
      injectRegister: 'script-defer',
      includeAssets: ['fonts/*.woff2', 'icon.svg'],
      manifest: {
        name: 'NithiDesk',
        short_name: 'NithiDesk',
        description: 'NithiDesk court diary',
        lang: 'en',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        background_color: '#f4f6f3',
        theme_color: '#1d3a31',
        icons: [
          { src: '/icon.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'any' },
          { src: '/icon-maskable.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'maskable' },
        ],
      },
      workbox: {
        // App shell only. API responses are never cached (ADR 0008).
        globPatterns: ['**/*.{js,css,html,woff2,svg}'],
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api\//, /^\/healthz/, /^\/readyz/, /^\/version/],
        runtimeCaching: [],
        cleanupOutdatedCaches: true,
      },
    }),
  ],
  build: {
    outDir: 'dist',
    sourcemap: false,
    target: 'es2020',
    assetsInlineLimit: 0,
  },
  server: {
    port: 5173,
    proxy: {
      // NITHIDESK_API overrides the API origin for local runs (default: the Go API on :8080).
      '/api': { target: process.env.NITHIDESK_API ?? 'http://localhost:8080', changeOrigin: false },
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
