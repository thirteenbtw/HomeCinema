import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// The Go server embeds the build from server/internal/webui/dist.
export default defineConfig({
  plugins: [react()],
  build: { outDir: '../server/internal/webui/dist', emptyOutDir: true, chunkSizeWarningLimit: 900 },
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://127.0.0.1:8096', changeOrigin: false } },
  },
});
