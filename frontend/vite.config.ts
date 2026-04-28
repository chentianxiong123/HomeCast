import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    vueDevTools(),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    },
  },
  server: {
    port: 28975,
    proxy: {
      '/api': {
        target: 'http://localhost:28974',
        changeOrigin: true,
        timeout: 120000,
        configure: (proxy, options) => {
          proxy.on('error', (err, req, res) => {
            console.error('Proxy error:', err.message, req.url)
          })
        },
      },
      '/proxy': {
        target: 'http://localhost:28974',
        changeOrigin: true,
        timeout: 120000,
      },
    },
  }
})
