import { defineConfig } from 'vite'
import preact from '@preact/preset-vite'

export default defineConfig({
  plugins: [preact()],
  server: {
    proxy: {
      '/trpc': {
        target: 'http://127.0.0.1:8787',
      },
      '/api/events': {
        target: 'http://127.0.0.1:8787',
      },
    },
  },
})
