import { defineConfig } from 'vite'
import preact from '@preact/preset-vite'

const gmuxdPort = process.env.VITE_DEV_PROXY_PORT || '8790'

export default defineConfig({
  plugins: [preact()],
  server: {
    allowedHosts: true,
    proxy: {
      '/v1': {
        target: `http://127.0.0.1:${gmuxdPort}`,
      },
      '/ws': {
        target: `http://127.0.0.1:${gmuxdPort}`,
        ws: true,
      },
    },
  },
})
