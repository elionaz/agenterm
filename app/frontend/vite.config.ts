import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  // Loaded from disk by the webview: one bundle is fine.
  build: { chunkSizeWarningLimit: 1024 },
})
