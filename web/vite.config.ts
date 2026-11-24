import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const backend = process.env.GDY_DEV_BACKEND ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [vue()],
  server: {
    host: '127.0.0.1',
    strictPort: true,
    proxy: {
      '/api': { target: backend, changeOrigin: false },
      '/ws': { target: backend, changeOrigin: false, ws: true },
    },
  },
})
